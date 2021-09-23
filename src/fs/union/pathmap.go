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

package union

// PATH MAP FILE FORMAT
//
// A file is a list of transactions.
//
//     file : transaction*
//
// A transaction is a list of chunks. A transaction is read into a temp path map. When all
// transaction chunks have been read and the transaction has been verified as valid, the temp
// path map is either assigned to the main path map (chunk command '=') or added to the main
// path map (chunk command '+'). A transaction is valid when all chunks are valid.
//
//     transaction : chunk*
//
// A chunk is a header followed by a list of records.
//
//     chunk : header record*
//
// A header is a structure that contains the signature "PATHMAP", a chunk command, and
// a cumulative crypto hash of the chunks' records. A header is 32 bytes long.
//
//     header : "PATHMAP" command cindex rcount hash
//
// A command instructs what to do with the chunk and is one of:
// - '.' Add records in chunk to temp path map.
// - '=' Add records in chunk to temp path map, assign temp path map to main path map,
// clear temp path map and complete transaction.
// - '+' Add records in chunk to temp path map, add temp path map to main path map,
// clear temp path map and complete transaction.
//
//     command : '.' | '=' | '+'
//
// An cindex contains the chunk index within a transaction (0-based, little-endian format).
//
//     cindex : byte[4]
//
// An rcount contains the record count of a chunk (little-endian format).
//
//     rcount : byte[4]
//
// A hash is a cumulative SHA256/128 crypto hash over the records of all prior chunks in the
// same transaction and this chunk's records.
//
//     hash : byte[16]
//
// A record is a structure that contains a path viskey and the path's parent viskey.
// The path viskey has the "dirty" bit set (dirty_viskey) so that it can be recognized
// as the beginning of a record. A record is 32 bytes long.
//
//     record : dirty_viskey viskey
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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
)

const Pathmapdbg = true

type Pathmap struct {
	vm   map[Pathkey]Pathkey              // visibility map
	hm   map[Pathkey]map[Pathkey]struct{} // hierarchy map
	fs   fuse.FileSystemInterface         // file system
	path string                           // path map file name
	fh   uint64                           // path map file handle
	ofs  int64                            // path map file offset
}

var _pathmapdbg map[Pathkey]string

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

// Function OpenPathmap opens a path map file on a file system and
// returns its in-memory representation.
func OpenPathmap(fs fuse.FileSystemInterface, path string) (int, *Pathmap) {
	pm := &Pathmap{
		vm:   make(map[Pathkey]Pathkey),
		hm:   make(map[Pathkey]map[Pathkey]struct{}),
		fs:   fs,
		path: path,
		fh:   ^uint64(0),
	}

	if Pathmapdbg {
		if nil == _pathmapdbg {
			_pathmapdbg = make(map[Pathkey]string)
		}
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

// Function Get returns opaqueness and visibility information.
// Visibility can be one of: unknown, whiteout, notexist, 0, 1, 2, ...
func (pm *Pathmap) Get(path string) (isopq bool, v uint8) {
	var k, p Pathkey
	var ok bool
	pkh := NewPathkeyHash()

	for i, j := 0, 0; ; {
		for j = i; len(path) > i && '/' == path[i]; i++ {
		}
		if j == i {
			break
		}
		if j == 0 {
			// root
			pkh.Write("/")
			k = pkh.ComputePathkey()
			p, ok = pm.vm[k]
			if !ok {
				return isopq, UNKNOWN
			}
		} else {
			// not root
			pkh.Write("/")
		}
		for j = i; len(path) > i && '/' != path[i]; i++ {
		}
		if j == i {
			break
		}
		pkh.Write(path[j:i])
		k = pkh.ComputePathkey()
		p, ok = pm.vm[k]
		if !ok {
			return isopq, UNKNOWN
		}
		isopq = isopq || OPAQUE == p[0]&_MASK
	}

	if !ok {
		return isopq, UNKNOWN
	}

	v = p[0] & _MASK
	if OPAQUE == v {
		v = 0
	}

	return
}

// Function Set sets visibility information.
// Visibility can be one of: opaque, whiteout, notexist, 0, 1, 2, ...
func (pm *Pathmap) Set(path string, v uint8) {
	if _MAXVIS < v {
		panic("invalid value")
	}

	var k, p Pathkey
	pkh := NewPathkeyHash()

	for i, j := 0, 0; ; {
		for j = i; len(path) > i && '/' == path[i]; i++ {
		}
		if j == i {
			break
		}
		if j == 0 {
			// root
			pkh.Write("/")
			k = pkh.ComputePathkey()
			if Pathmapdbg {
				_pathmapdbg[k] = "/"
			}
			if len(path) > i {
				pm.set(k, p, UNKNOWN)
			} else {
				pm.set(k, p, v)
			}
			p = k
		} else {
			// not root
			p = k
			pkh.Write("/")
		}
		for j = i; len(path) > i && '/' != path[i]; i++ {
		}
		if j == i {
			break
		}
		pkh.Write(path[j:i])
		k = pkh.ComputePathkey()
		if Pathmapdbg {
			_pathmapdbg[k] = path[:i]
		}
		if len(path) > i {
			pm.set(k, p, UNKNOWN)
		} else {
			pm.set(k, p, v)
		}
	}
}

// Function SetTree sets visibility information for a whole tree.
// Visibility can be one of: opaque, whiteout, notexist, 0, 1, 2, ...
func (pm *Pathmap) SetTree(path string, rootv, v uint8) {
	if _MAXVIS < rootv || _MAXVIS < v {
		panic("invalid value")
	}

	pm.settree(ComputePathkey(path), rootv, v)
}

func (pm *Pathmap) set(k, p Pathkey, v uint8) {
	u := _DIRT | UNKNOWN
	if q, ok := pm.vm[k]; ok {
		u = q[0]
		q[0] = 0
		if p != q {
			panic("wrong parent hash")
		}
		if UNKNOWN == v && NOTEXIST != u&_MASK {
			v = u & _MASK
		}
	}

	p[0] = 0
	m := pm.hm[p]
	if nil == m {
		m = make(map[Pathkey]struct{})
		pm.hm[p] = m
	}
	m[k] = struct{}{}

	p[0] = _pathmap_newv(u, v)
	pm.vm[k] = p
}

func (pm *Pathmap) settree(k Pathkey, rootv, v uint8) {
	p, ok := pm.vm[k]
	if !ok {
		return
	}

	p[0] = _pathmap_newv(p[0], rootv)
	pm.vm[k] = p

	for k := range pm.hm[k] {
		pm.settree(k, v, v)
	}
}

func _pathmap_newv(u uint8, v uint8) uint8 {
	dirt := u & _DIRT
	if 0 == dirt {
		// Set _dirt bit if visibility "kind" changes.
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

// Function Enum enumerates visibility information for a directory.
func (pm *Pathmap) Enum(path string, fn func(k Pathkey, v uint8)) {
	pm.enum(ComputePathkey(path), fn)
}

func (pm *Pathmap) enum(k Pathkey, fn func(k Pathkey, v uint8)) {
	for k := range pm.hm[k] {
		p, ok := pm.vm[k]
		if !ok {
			continue
		}

		v := p[0] & _MASK
		if OPAQUE == v {
			v = 0
		}

		fn(k, v)
	}
}

func (pm *Pathmap) read() int {
	rdr := bufio.NewReaderSize(
		&_pathmaprdr{fs: pm.fs, path: pm.path, fh: pm.fh, ofs: pm.ofs},
		4096*2*Pathkeylen)

	for {
		n := pm.read_transaction(rdr)
		if 0 > n {
			return n
		}
		if 0 == n {
			break
		}
	}

	pm.hm = make(map[Pathkey]map[Pathkey]struct{})
	for k, p := range pm.vm {
		p[0] = 0
		m := pm.hm[p]
		if nil == m {
			m = make(map[Pathkey]struct{})
			pm.hm[p] = m
		}
		m[k] = struct{}{}
	}

	return 1
}

func (pm *Pathmap) read_transaction(rdr *bufio.Reader) int {
	tmp := make(map[Pathkey]Pathkey)
	hsh := sha256.New()
	cmd := uint8(0)
	chi := uint32(0)
	idx := uint32(0)
	cnt := uint32(0)
	equ := true

	var rec [2 * Pathkeylen]uint8
	var sum [16]uint8
	var k, p Pathkey

	for {
		for {
			n := _pathmap_read(rdr, rec[:])
			if 0 >= n {
				return n
			}
			pm.ofs += int64(len(rec))

			cmd = rec[7]
			if "PATHMAP" == string(rec[:7]) && ('=' == cmd || '+' == cmd || '.' == cmd) {
				if 0 == binary.LittleEndian.Uint32(rec[8:]) {
					tmp = make(map[Pathkey]Pathkey)
					hsh.Reset()
					chi = uint32(0)
				} else {
					equ = equ && (chi == binary.LittleEndian.Uint32(rec[8:]))
				}
				chi++
				break
			}
		}

		cnt = binary.LittleEndian.Uint32(rec[12:])
		copy(sum[:], rec[Pathkeylen:])

		for idx = 0; cnt > idx; idx++ {
			n := _pathmap_read(rdr, rec[:1])
			if 0 >= n {
				return n
			}
			if 0 == rec[0]&_DIRT {
				rdr.UnreadByte()
				break
			}
			n = _pathmap_read(rdr, rec[1:])
			if 0 >= n {
				return n
			}
			pm.ofs += int64(len(rec))

			hsh.Write(rec[:])
			copy(k[:], rec[:Pathkeylen])
			copy(p[:], rec[Pathkeylen:])

			k[0] &^= _DIRT // clear _dirt bit used to ensure non-zero record

			tmp[k] = p
		}

		equ = equ && (cnt == idx && bytes.Equal(sum[:], hsh.Sum(nil)[:Pathkeylen]))

		if '=' == cmd || '+' == cmd {
			if equ {
				if '=' == cmd {
					pm.vm = make(map[Pathkey]Pathkey)
				}
				for k, p = range tmp {
					switch p[0] {
					case UNKNOWN, WHITEOUT, OPAQUE:
						// insert record: add key to map
						pm.vm[k] = p
					case 0:
						// delete record: delete key from map
						delete(pm.vm, k)
					}
				}
			}
			return 1
		}
	}
}

func _pathmap_read(rdr *bufio.Reader, rec []byte) int {
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

// Function Write writes the path map to the associated file on the file system.
func (pm *Pathmap) Write() int {
	if nil == pm.fs {
		return -fuse.EPERM
	}

	count := int(pm.ofs / (2 * Pathkeylen))

	if 1024 < count && 2*len(pm.vm) < count {
		n := pm.write_transaction(false, pm.ofs)
		if 0 > n {
			return n
		}

		return pm.write_transaction(false, 0)
	} else {
		return pm.write_transaction(true, pm.ofs)
	}
}

func (pm *Pathmap) write_transaction(incremental bool, ofs0 int64) int {
	truncate := !incremental && 0 == ofs0

	buf := make([]byte, 4096*2*Pathkeylen)
	ptr := 2 * Pathkeylen
	chi := uint32(0)
	cnt := uint32(0)
	hsh := sha256.New()
	ofs := ofs0

	write := func(cmd uint8) int {
		hsh.Write(buf[2*Pathkeylen : ptr])
		copy(buf[:], "PATHMAP")
		buf[7] = cmd
		binary.LittleEndian.PutUint32(buf[8:], chi)
		binary.LittleEndian.PutUint32(buf[12:], cnt)
		copy(buf[Pathkeylen:2*Pathkeylen], hsh.Sum(nil))

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

	for k, p := range pm.vm {
		if incremental && 0 == p[0]&_DIRT {
			continue
		}

		p[0] &= _MASK
		if _MAXIDX >= p[0] && 0 != len(pm.hm[k]) {
			// keep record for dirs with children (but exclude notexist)
			p[0] = UNKNOWN
		}

		switch p[0] {
		case UNKNOWN, WHITEOUT, OPAQUE:
			// insert record: add key to map
		default:
			if !incremental {
				continue
			}
			// delete record: delete key from map
			p[0] = 0
		}

		k[0] |= _DIRT // set _dirt to ensure non-zero record

		if len(buf) <= ptr {
			n := write('.')
			if 0 > n {
				return n
			}

			ptr = 2 * Pathkeylen
			chi++
			cnt = uint32(0)
		}

		copy(buf[ptr:], k[:])
		copy(buf[ptr+Pathkeylen:], p[:])

		ptr += 2 * Pathkeylen
		cnt++
	}

	if 2*Pathkeylen < ptr {
		n := 0
		if incremental {
			n = write('+')
		} else {
			n = write('=')
		}
		if 0 > n {
			return n
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

	for k, p := range pm.vm {
		if incremental && 0 == p[0]&_DIRT {
			continue
		}

		p[0] &= _MASK
		pm.vm[k] = p
	}

	return 1
}

// Function Purge purges non-persistent and non-dirty entries from the path map.
func (pm *Pathmap) Purge() {
	for k, p := range pm.vm {
		if 0 != p[0]&_DIRT {
			continue
		}

		if _MAXIDX >= p[0] && 0 != len(pm.hm[k]) {
			// keep record for dirs with children (but exclude notexist)
			p[0] = UNKNOWN
		}

		switch p[0] {
		case UNKNOWN, WHITEOUT, OPAQUE:
			// keep record
		default:
			delete(pm.vm, k)
			delete(pm.hm, k)
			p[0] = 0
			if m, ok := pm.hm[p]; ok {
				delete(m, k)
				if 0 == len(m) {
					delete(pm.hm, p)
				}
			}
		}
	}
}

// Function Dump dumps the path map for diagnostic purposes.
func (pm *Pathmap) Dump() {
	path := "/"
	recursive := true

	level := 0
	var dump func(k Pathkey, v uint8)
	dump = func(k Pathkey, v uint8) {
		p := pm.vm[k]

		dirt, hier := '-', '-'
		if 0 != _DIRT&p[0] {
			dirt = 'D'
		}
		if _, ok := pm.hm[k]; ok {
			hier = 'H'
		}

		vstr := ""
		switch p[0] & _MASK {
		case UNKNOWN:
			vstr = "unknown"
		case OPAQUE:
			vstr = "opaque"
		case WHITEOUT:
			vstr = "whiteout"
		case NOTEXIST:
			vstr = "notexist"
		default:
			vstr = fmt.Sprint(p[0] & _MASK)
		}

		fmt.Printf("%c%c %-13s%*s%s :: %s\n",
			hier, dirt, vstr, level*2, "", _pathmap_ktoa(k), _pathmap_ktoa(p))

		if recursive {
			level++
			pm.enum(k, dump)
			level--
		}
	}

	dump(ComputePathkey(path), 0)
	fmt.Printf("len(vm)=%v\n", len(pm.vm))
}

func (pm *Pathmap) DumpFile() int {
	if nil == pm.fs {
		return -fuse.EPERM
	}

	rdr := bufio.NewReaderSize(
		&_pathmaprdr{fs: pm.fs, path: pm.path, fh: pm.fh, ofs: 0},
		4096*2*Pathkeylen)

	hsh := sha256.New()
	cmd := uint8(0)
	chi := uint32(0)
	idx := uint32(0)
	cnt := uint32(0)
	equ := true

	var rec [2 * Pathkeylen]uint8
	var sum [16]uint8
	var k, p Pathkey

	for {
		for {
			n := _pathmap_read(rdr, rec[:])
			if 0 >= n {
				return n
			}

			cmd = rec[7]
			if "PATHMAP" == string(rec[:7]) && ('=' == cmd || '+' == cmd || '.' == cmd) {
				if 0 == binary.LittleEndian.Uint32(rec[8:]) {
					hsh.Reset()
					chi = uint32(0)
				} else {
					equ = equ && (chi == binary.LittleEndian.Uint32(rec[8:]))
				}
				chi++
				break
			}
		}

		cnt = binary.LittleEndian.Uint32(rec[12:])
		copy(sum[:], rec[Pathkeylen:])

		fmt.Printf("%s chi=%v cnt=%v sum=", string(rec[:8]), chi-1, cnt)
		for _, x := range sum {
			fmt.Printf("%02x", x)
		}
		fmt.Println()

		for idx = 0; cnt > idx; idx++ {
			n := _pathmap_read(rdr, rec[:1])
			if 0 >= n {
				return n
			}
			if 0 == rec[0]&_DIRT {
				rdr.UnreadByte()
				break
			}
			n = _pathmap_read(rdr, rec[1:])
			if 0 >= n {
				return n
			}

			hsh.Write(rec[:])
			copy(k[:], rec[:Pathkeylen])
			copy(p[:], rec[Pathkeylen:])

			vstr := ""
			switch p[0] {
			case UNKNOWN:
				vstr = "unknown"
			case OPAQUE:
				vstr = "opaque"
			case WHITEOUT:
				vstr = "whiteout"
			default:
				vstr = fmt.Sprint(p[0])
			}
			fmt.Printf("%-13s%s :: %s\n", vstr, _pathmap_ktoa(k), _pathmap_ktoa(p))
		}

		equ = equ && (cnt == idx && bytes.Equal(sum[:], hsh.Sum(nil)[:Pathkeylen]))

		fmt.Printf("%c equ=%v\n", cmd, equ)
		fmt.Println()
	}
}

// Function SanityCheck checks the path map for internal consistency.
func (pm *Pathmap) SanityCheck() error {
	errs := []string{}

	count := 0
	var check func(k Pathkey, v uint8)
	check = func(k Pathkey, v uint8) {
		p := pm.vm[k]

		q := p
		q[0] = 0

		if _, ok := pm.hm[q]; !ok {
			errs = append(errs,
				fmt.Sprintf("_, ok := pm.hm[q]; !ok # k=%v", _pathmap_ktoa(k)))
		}

		count++

		pm.enum(k, check)
	}

	check(ComputePathkey("/"), 0)

	if len(pm.vm) != count {
		errs = append(errs,
			fmt.Sprintf("len(vm) != count # %v != %v", len(pm.vm), count))
	}

	if 0 != len(errs) {
		return errors.New(strings.Join(errs, "\n"))
	}

	return nil
}

func _pathmap_ktoa(k Pathkey) string {
	if Pathmapdbg {
		q := k
		q[0] = 0
		if path, ok := _pathmapdbg[q]; ok {
			return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x (%s)",
				k[0], k[1], k[2], k[3], k[4], k[5], k[6], k[7], path)
		}
	}
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x",
		k[0], k[1], k[2], k[3], k[4], k[5], k[6], k[7])
}

type _pathmaprdr struct {
	fs   fuse.FileSystemInterface
	path string
	fh   uint64
	ofs  int64
}

func (rdr *_pathmaprdr) Read(p []uint8) (n int, err error) {
	n = rdr.fs.Read(rdr.path, p, rdr.ofs, rdr.fh)
	if 0 > n {
		return 0, fuse.Error(n)
	} else if 0 == n {
		return 0, io.EOF
	}
	rdr.ofs += int64(n)
	return n, nil
}
