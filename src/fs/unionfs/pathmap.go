/*
 * pathmap.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package unionfs

// PATH MAP FILE FORMAT
//
// A file is a list of transactions.
//
//     file : transaction*
//
// A transaction is a list of chunks. A transaction is read into a temp path map. When all
// transaction chunks have been read and the transaction has been verified as valid, the temp
// path map is either assigned to the main path map (chunk command 'S') or added to the main
// path map (chunk command 'A'). A transaction is valid when all chunks are valid.
//
//     transaction : chunk*
//
// A chunk is a header followed by a list of records.
//
//     chunk : header record*
//
// A header is a structure that contains a chunk indicator, a chunk command, a count of the
// records in the chunk, and a cumulative crypto hash of the records in the chunk and any
// previous chunks. A header is 16 bytes long.
//
//     header : indicator command rcount hash
//
// A chunk indicator is either '1' for the first chunk or '0 for all chunks after the first.
//
//     indicator : '1' | '0'
//
// A command instructs what to do with the chunk and is one of:
// - 'P' Add records in chunk to temp path map.
// - 'S' Add records in chunk to temp path map, assign temp path map to main path map,
// clear temp path map and complete transaction.
// - 'A' Add records in chunk to temp path map, add temp path map to main path map,
// clear temp path map and complete transaction.
//
//     command : 'P' | 'S' | 'A'
//
// An rcount contains the record count of a chunk (little-endian format).
//
//     rcount : byte[2]
//
// A hash is a cumulative SHA256/96 crypto hash over the records of all prior chunks in the
// same transaction and this chunk's records.
//
//     hash : byte[12]
//
// A record is a path key and is 16 bytes long. The first byte in the path key has the "dirty"
// bit (bit with value 0x80) set, so that is can be recognized as the beginning of a record.
//
//     record : byte[16]
//
// Another way to look at a file is to see it as simply a list of headers and records. Headers
// have the "dirty" bit (bit with value 0x80) always clear (0). Records have the "dirty" bit
// (bit with value 0x80) always set (1). Therefore it is easy to identify a header and the
// beginning of a chunk when recovering from a failed transaction commit.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/billziss-gh/cgofuse/fuse"
)

type Pathmap struct {
	Caseins bool
	vm      map[Pathkey]uint8        // visibility map
	fs      fuse.FileSystemInterface // file system
	path    string                   // path map file name
	fh      uint64                   // path map file handle
	ofs     int64                    // path map file offset
}

const (
	_DIRT    = uint8(0x80)
	_MASK    = uint8(0x7f)
	UNKNOWN  = _MASK - 0
	OPAQUE   = _MASK - 1
	WHITEOUT = _MASK - 2
	NOTEXIST = _MASK - 3
	_MAXVIS  = OPAQUE
	_MAXIDX  = NOTEXIST - 1
)

const pathmapdbg = false

var pathmapdbgMap map[Pathkey]string

// Function OpenPathmap opens a path map file on a file system and
// returns its in-memory representation.
func OpenPathmap(fs fuse.FileSystemInterface, path string, caseins bool) (int, *Pathmap) {
	pm := &Pathmap{
		Caseins: caseins,
		vm:      make(map[Pathkey]uint8),
		fs:      fs,
		path:    path,
		fh:      ^uint64(0),
	}

	if nil != pm.fs {
		var errc int
		errc, pm.fh = fs.Open(path, fuse.O_RDWR)
		if 0 != errc {
			errc, pm.fh = fs.Create(path, fuse.O_CREAT|fuse.O_RDWR, 0600)
			if -fuse.ENOSYS == errc {
				errc = fs.Mknod(path, 0600, 0)
				if 0 == errc {
					errc, pm.fh = fs.Open(path, fuse.O_RDWR)
				}
			}
			if 0 != errc {
				return errc, nil
			}
		}

		n := pm.read()
		if 0 > n {
			return n, nil
		}
	}

	return 0, pm
}

// Function Close closes a path map.
func (pm *Pathmap) Close() {
	if nil != pm.fs {
		pm.fs.Release(pm.path, pm.fh)
	}
	*pm = Pathmap{}
}

// Function Get returns opaqueness and visibility information for a path.
// Visibility can be one of: unknown, whiteout, notexist, 0, 1, 2, ...
func (pm *Pathmap) Get(path string) (isopq bool, v uint8) {
	var ok bool
	pkh := NewPathkeyHash(pm.Caseins)

	for i, j := 0, 0; ; {
		for j = i; len(path) > i && '/' == path[i]; i++ {
		}
		if j == i {
			break
		}
		pkh.Write(path[j:i])
		if j == 0 {
			if v, ok = pm.vm[pkh.ComputePathkey()]; ok {
				isopq = isopq || OPAQUE == v&_MASK
			}
		}
		for j = i; len(path) > i && '/' != path[i]; i++ {
		}
		if j == i {
			break
		}
		pkh.Write(path[j:i])
		if v, ok = pm.vm[pkh.ComputePathkey()]; ok {
			isopq = isopq || OPAQUE == v&_MASK
		}
	}

	if !ok {
		return isopq, UNKNOWN
	}

	v &= _MASK
	if OPAQUE == v {
		v = 0
	}

	return
}

// Function Has returns if visibility information exists for a path.
func (pm *Pathmap) Has(path string) (ok bool) {
	k := ComputePathkey(path, pm.Caseins)
	_, ok = pm.vm[k]

	return
}

// Function IsDirty determines if a path is "dirty"
// (i.e. it has visibility information changes that have not been written).
func (pm *Pathmap) IsDirty(path string) (dirt bool) {
	k := ComputePathkey(path, pm.Caseins)
	v, ok := pm.vm[k]
	if ok {
		dirt = 0 != v&_DIRT
	}

	return
}

// Function Set sets visibility information for path.
// Visibility can be one of: opaque, whiteout, notexist, 0, 1, 2, ...
func (pm *Pathmap) Set(path string, v uint8) {
	if _MAXVIS < v {
		panic("invalid value")
	}

	k := ComputePathkey(path, pm.Caseins)
	u, ok := pm.vm[k]
	if !ok {
		u = UNKNOWN

		if pathmapdbg {
			if nil == pathmapdbgMap {
				pathmapdbgMap = make(map[Pathkey]string)
			}
			pathmapdbgMap[k] = path
		}
	}

	pm.vm[k] = _pathmapNewv(u, v)
}

// Function SetIf sets visibility information for a path only if some already exists.
// Visibility can be one of: opaque, whiteout, notexist, 0, 1, 2, ...
func (pm *Pathmap) SetIf(path string, v uint8) {
	if _MAXVIS < v {
		panic("invalid value")
	}

	k := ComputePathkey(path, pm.Caseins)
	u, ok := pm.vm[k]
	if !ok {
		return
	}

	pm.vm[k] = _pathmapNewv(u, v)
}

// Function read reads the path map file and applies all transactions in it.
func (pm *Pathmap) read() int {
	rdr := bufio.NewReaderSize(
		&_pathmapReader{fs: pm.fs, path: pm.path, fh: pm.fh, ofs: pm.ofs},
		4096*Pathkeylen)

	for {
		n := pm.readTransaction(rdr)
		if 0 > n {
			return n
		}
		if 0 == n {
			break
		}
	}

	return 1
}

// Function readTransaction reads a single transaction.
// It returns a negative error code on error, 0 on EOF, 1 when transaction is found
// (regardless if it was applied or not).
func (pm *Pathmap) readTransaction(rdr *bufio.Reader) int {
	tmp := make(map[Pathkey]uint8)
	hsh := sha256.New()
	ch1 := false
	cmd := uint8(0)
	idx := uint16(0)
	cnt := uint16(0)
	equ := true

	var k Pathkey
	var sum [12]uint8

	for {
		for {
			n := _pathmapRead(rdr, k[:1])
			if 0 >= n {
				return n
			}
			if ch1 && '1' == k[0] {
				// found unexpected chunk 1; abort transaction
				rdr.UnreadByte()
				return 1
			}
			n = _pathmapRead(rdr, k[1:])
			if 0 >= n {
				return n
			}
			pm.ofs += Pathkeylen

			cmd = k[1]
			if !ch1 {
				if '1' == k[0] && ('P' == cmd || 'S' == cmd || 'A' == cmd) {
					// found chunk 1; process it and expect chunk not-1
					ch1 = true
					break
				} else {
					// found trash; loop until chunk 1
					continue
				}
			} else {
				if '0' == k[0] && ('P' == cmd || 'S' == cmd || 'A' == cmd) {
					// found chunk not-1; process it
					break
				} else {
					// found trash; abort transaction
					return 1
				}
			}
		}

		cnt = binary.LittleEndian.Uint16(k[2:])
		copy(sum[:], k[4:])

		for idx = 0; cnt > idx; idx++ {
			n := _pathmapRead(rdr, k[:1])
			if 0 >= n {
				return n
			}
			if 0 == k[0]&_DIRT {
				rdr.UnreadByte()
				break
			}
			n = _pathmapRead(rdr, k[1:])
			if 0 >= n {
				return n
			}
			pm.ofs += Pathkeylen

			hsh.Write(k[:])
			v := k[0] & _MASK // clear _DIRT bit used to ensure non-zero record
			k[0] = 0
			tmp[k] = v
		}

		equ = equ && (cnt == idx && bytes.Equal(sum[:], hsh.Sum(nil)[:len(sum)]))

		if 'S' == cmd || 'A' == cmd {
			if equ {
				if 'S' == cmd {
					pm.vm = make(map[Pathkey]uint8)
				}
				for k, v := range tmp {
					switch v {
					case WHITEOUT, OPAQUE:
						// insert record: add key to map
						pm.vm[k] = v
					case NOTEXIST:
						// delete record: delete key from map
						delete(pm.vm, k)
					}
				}
			}
			return 1
		}
	}
}

// Function Write writes the path map to the associated file on the file system.
func (pm *Pathmap) Write() int {
	if nil == pm.fs {
		return -fuse.EPERM
	}

	count := int(pm.ofs / Pathkeylen)

	if 1024 < count && 2*len(pm.vm) < count {
		n := pm.writeTransaction(false, pm.ofs)
		if 0 > n {
			return n
		}

		return pm.writeTransaction(false, 0)
	} else {
		return pm.writeTransaction(true, pm.ofs)
	}
}

// Function writeTransaction writes a single transaction.
func (pm *Pathmap) writeTransaction(incremental bool, ofs0 int64) int {
	truncate := !incremental && 0 == ofs0

	buf := make([]byte, 4096*Pathkeylen)
	hsh := sha256.New()
	ptr := Pathkeylen
	chi := uint8('1')
	cnt := uint16(0)
	ofs := ofs0

	write := func(cmd uint8) int {
		hsh.Write(buf[Pathkeylen:ptr])
		buf[0] = chi
		buf[1] = cmd
		binary.LittleEndian.PutUint16(buf[2:], cnt)
		copy(buf[4:Pathkeylen], hsh.Sum(nil))

		n := pm.fs.Write(pm.path, buf[:ptr], ofs, pm.fh)
		if 0 > n {
			return n
		}
		if ptr != n {
			return -fuse.EIO
		}
		ofs += int64(n)
		return n
	}

	for k, v := range pm.vm {
		if incremental && 0 == v&_DIRT {
			continue
		}

		v &= _MASK
		switch v {
		case WHITEOUT, OPAQUE:
			// insert record: add key to map
		default:
			if !incremental {
				continue
			}
			// delete record: delete key from map
			v = NOTEXIST
		}

		if len(buf) <= ptr {
			if n := write('P'); 0 > n {
				return n
			}

			ptr = Pathkeylen
			chi = uint8('0')
			cnt = uint16(0)
		}

		k[0] = _DIRT | v // set _DIRT to ensure non-zero record
		copy(buf[ptr:], k[:])

		ptr += Pathkeylen
		cnt++
	}

	if Pathkeylen < ptr {
		if incremental {
			if n := write('A'); 0 > n {
				return n
			}
		} else {
			if n := write('S'); 0 > n {
				return n
			}
		}
	}

	if ofs == ofs0 {
		return 0
	}

	errc := pm.fs.Fsync(pm.path, true, pm.fh)
	if 0 != errc && -fuse.ENOSYS != errc {
		return errc
	}

	if truncate {
		errc := pm.fs.Truncate(pm.path, ofs, pm.fh)
		if 0 != errc {
			return errc
		}

		errc = pm.fs.Fsync(pm.path, true, pm.fh)
		if 0 != errc && -fuse.ENOSYS != errc {
			return errc
		}
	}

	pm.ofs = ofs

	for k, v := range pm.vm {
		if incremental && 0 == v&_DIRT {
			continue
		}

		pm.vm[k] = v & _MASK
	}

	return 1
}

// Function Purge purges non-persistent and non-dirty entries from the path map.
func (pm *Pathmap) Purge() {
	for k, v := range pm.vm {
		if 0 != v&_DIRT {
			continue
		}

		switch v {
		case WHITEOUT, OPAQUE:
			// keep record
		default:
			delete(pm.vm, k)
		}
	}
}

// Function Dump dumps the path map file for diagnostic purposes.
func (pm *Pathmap) Dump(dmp io.Writer) int {
	if nil == pm.fs {
		return -fuse.EPERM
	}

	rdr := bufio.NewReaderSize(
		&_pathmapReader{fs: pm.fs, path: pm.path, fh: pm.fh, ofs: 0},
		4096*Pathkeylen)

	for ofs := uint64(0); ; {
		n := pm.dumpTransaction(rdr, &ofs, dmp)
		if 0 > n {
			return n
		}
		if 0 == n {
			break
		}
	}

	return 1
}

// Function dumpTransaction dumps a single transaction.
func (pm *Pathmap) dumpTransaction(rdr *bufio.Reader, pofs *uint64, dmp io.Writer) int {
	hsh := sha256.New()
	ch1 := false
	cmd := uint8(0)
	idx := uint16(0)
	cnt := uint16(0)
	equ := true

	var k Pathkey
	var sum [12]uint8

	for {
		for {
			n := _pathmapRead(rdr, k[:1])
			if 0 >= n {
				return n
			}
			if ch1 && '1' == k[0] {
				// found unexpected chunk 1; abort transaction
				rdr.UnreadByte()
				return 1
			}
			n = _pathmapRead(rdr, k[1:])
			if 0 >= n {
				return n
			}
			*pofs += Pathkeylen

			cmd = k[1]
			if !ch1 {
				if '1' == k[0] && ('P' == cmd || 'S' == cmd || 'A' == cmd) {
					// found chunk 1; process it and expect chunk not-1
					ch1 = true
					break
				} else {
					// found trash; loop until chunk 1
					continue
				}
			} else {
				if '0' == k[0] && ('P' == cmd || 'S' == cmd || 'A' == cmd) {
					// found chunk not-1; process it
					break
				} else {
					// found trash; abort transaction
					return 1
				}
			}
		}

		cnt = binary.LittleEndian.Uint16(k[2:])
		copy(sum[:], k[4:])

		fmt.Fprintf(dmp,
			"%c%c cnt=%v hash=%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x%02x (ofs=%08x)\n",
			k[0], cmd, cnt,
			sum[0], sum[1], sum[2], sum[3],
			sum[4], sum[5], sum[6], sum[7],
			sum[8], sum[9], sum[10], sum[11],
			*pofs-Pathkeylen)

		for idx = 0; cnt > idx; idx++ {
			n := _pathmapRead(rdr, k[:1])
			if 0 >= n {
				return n
			}
			if 0 == k[0]&_DIRT {
				rdr.UnreadByte()
				break
			}
			n = _pathmapRead(rdr, k[1:])
			if 0 >= n {
				return n
			}
			*pofs += Pathkeylen

			hsh.Write(k[:])
			v := k[0] & _MASK // clear _DIRT bit used to ensure non-zero record

			vstr := ""
			switch v {
			case UNKNOWN:
				vstr = "unknown"
			case OPAQUE:
				vstr = "opaque"
			case WHITEOUT:
				vstr = "whiteout"
			case NOTEXIST:
				vstr = "notexist"
			default:
				vstr = fmt.Sprint(v)
			}

			fmt.Fprintf(dmp, "- %-13s %s\n", vstr, _pathmapKtoa(k))
		}

		equ = equ && (cnt == idx && bytes.Equal(sum[:], hsh.Sum(nil)[:len(sum)]))

		if equ {
			fmt.Fprintf(dmp, "COMMIT\n\n")
		} else {
			fmt.Fprintf(dmp, "ABORT\n\n")
		}

		if 'S' == cmd || 'A' == cmd {
			return 1
		}
	}
}

func _pathmapNewv(u uint8, v uint8) uint8 {
	dirt := u & _DIRT
	if 0 == dirt {
		// Set _DIRT bit if visibility "kind" changes.
		// Kind is one of: unknown/"index", opaque, whiteout, notexist
		ukind := u & _MASK
		vkind := v & _MASK
		if _MAXIDX >= ukind {
			ukind = UNKNOWN
		}
		if _MAXIDX >= vkind {
			vkind = UNKNOWN
		}
		if ukind != vkind {
			dirt = _DIRT
		}
	}

	return dirt | v
}

func _pathmapKtoa(k Pathkey) string {
	if nil != pathmapdbgMap {
		q := k
		q[0] = 0
		if path, ok := pathmapdbgMap[q]; ok {
			return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x (%s)",
				k[0], k[1], k[2], k[3], k[4], k[5], k[6], k[7], path)
		}
	}
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x",
		k[0], k[1], k[2], k[3], k[4], k[5], k[6], k[7])
}

func _pathmapRead(rdr *bufio.Reader, rec []byte) int {
	n, err := rdr.Read(rec)
	if io.EOF == err {
		return 0
	} else if nil != err {
		if e, ok := err.(fuse.Error); ok {
			return int(e)
		} else {
			return -fuse.EIO
		}
	} else if len(rec) != n {
		return 0
	}
	return n
}

type _pathmapReader struct {
	fs   fuse.FileSystemInterface
	path string
	fh   uint64
	ofs  int64
}

func (rdr *_pathmapReader) Read(p []uint8) (n int, err error) {
	n = rdr.fs.Read(rdr.path, p, rdr.ofs, rdr.fh)
	if 0 > n {
		return 0, fuse.Error(n)
	} else if 0 == n {
		return 0, io.EOF
	}
	rdr.ofs += int64(n)
	return n, nil
}
