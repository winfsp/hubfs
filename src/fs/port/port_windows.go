// +build windows

/*
 * port_windows.go
 *
 * Copyright 2017-2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package port

import (
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/billziss-gh/cgofuse/fuse"
)

func Chdir(path string) (errc int) {
	pathu16, e := syscall.UTF16PtrFromString(path)
	if nil != e {
		return Errno(e)
	}

	return Errno(syscall.SetCurrentDirectory(pathu16))
}

func Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	pathu16, e := syscall.UTF16PtrFromString(path)
	if nil != e {
		return Errno(e)
	}

	var VolumeSerialNumber,
		MaxComponentLength,
		SectorsPerCluster,
		BytesPerSector,
		NumberOfFreeClusters,
		TotalNumberOfClusters uint32

	var rootu16 [260]uint16
	r1, _, e := syscall.Syscall(
		getVolumePathNameW.Addr(),
		3,
		uintptr(unsafe.Pointer(pathu16)),
		uintptr(unsafe.Pointer(&rootu16)),
		uintptr(len(rootu16)))
	if 0 == r1 {
		return Errno(e)
	}

	r1, _, e = syscall.Syscall9(
		getVolumeInformationW.Addr(),
		8,
		uintptr(unsafe.Pointer(&rootu16)),
		0,
		0,
		uintptr(unsafe.Pointer(&VolumeSerialNumber)),
		uintptr(unsafe.Pointer(&MaxComponentLength)),
		0,
		0,
		0,
		0)
	if 0 == r1 {
		return Errno(e)
	}

	r1, _, e = syscall.Syscall6(
		getDiskFreeSpaceW.Addr(),
		5,
		uintptr(unsafe.Pointer(&rootu16)),
		uintptr(unsafe.Pointer(&SectorsPerCluster)),
		uintptr(unsafe.Pointer(&BytesPerSector)),
		uintptr(unsafe.Pointer(&NumberOfFreeClusters)),
		uintptr(unsafe.Pointer(&TotalNumberOfClusters)),
		0)
	if 0 == r1 {
		return Errno(e)
	}

	*stat = fuse.Statfs_t{}
	stat.Bsize = uint64(SectorsPerCluster) * uint64(BytesPerSector)
	stat.Frsize = uint64(SectorsPerCluster) * uint64(BytesPerSector)
	stat.Blocks = uint64(TotalNumberOfClusters)
	stat.Bfree = uint64(NumberOfFreeClusters)
	stat.Bavail = uint64(TotalNumberOfClusters)
	stat.Fsid = uint64(VolumeSerialNumber)
	stat.Namemax = uint64(MaxComponentLength)

	return 0
}

func Mknod(path string, mode uint32, dev int) (errc int) {
	return -fuse.ENOSYS
}

func Mkdir(path string, mode uint32) (errc int) {
	pathu16, e := syscall.UTF16PtrFromString(path)
	if nil != e {
		return Errno(e)
	}

	return Errno(syscall.CreateDirectory(pathu16, nil))
}

func Unlink(path string) (errc int) {
	errc, fh := open(path, 0x00010100 /*DELETE | FILE_WRITE_ATTRIBUTES*/, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		type FILE_DISPOSITION_INFO struct {
			Flags uint32
		}

		info := FILE_DISPOSITION_INFO{}
		info.Flags = 0x13 /*DELETE | POSIX_SEMANTICS | IGNORE_READONLY_ATTRIBUTE*/

		r1, _, e := syscall.Syscall6(
			setFileInformationByHandle.Addr(),
			4,
			uintptr(fh),
			21, /*FileDispositionInfoEx*/
			uintptr(unsafe.Pointer(&info)),
			uintptr(unsafe.Sizeof(info)),
			0,
			0)
		if 0 == r1 && 87 /*ERROR_INVALID_PARAMETER*/ == e {
			info.Flags = 1 /*DeleteFile*/
			r1, _, e = syscall.Syscall6(
				setFileInformationByHandle.Addr(),
				4,
				uintptr(fh),
				4, /*FileDispositionInfo*/
				uintptr(unsafe.Pointer(&info)),
				uintptr(unsafe.Sizeof(info)),
				0,
				0)
		}
		if 0 == r1 {
			errc = Errno(e)
		}

		close(fh)
	}

	return
}

func Rmdir(path string) (errc int) {
	return Unlink(path)
}

func Link(oldpath string, newpath string) (errc int) {
	return -fuse.ENOSYS
}

func Symlink(oldpath string, newpath string) (errc int) {
	oldpath = strings.ReplaceAll(oldpath, `/`, `\`)
	oldpathu16, e := syscall.UTF16PtrFromString(oldpath)
	if nil != e {
		return Errno(e)
	}

	newpathu16, e := syscall.UTF16PtrFromString(newpath)
	if nil != e {
		return Errno(e)
	}

	return Errno(syscall.CreateSymbolicLink(
		newpathu16,
		oldpathu16,
		3 /*DIRECTORY | ALLOW_UNPRIVILEGED_CREATE*/))
}

func Readlink(path string) (errc int, target string) {
	errc, fh := open(path, 0, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		tmp := [syscall.MAXIMUM_REPARSE_DATA_BUFFER_SIZE]uint8{}

		var bytes uint32
		errc = Errno(syscall.DeviceIoControl(
			syscall.Handle(fh),
			syscall.FSCTL_GET_REPARSE_POINT,
			nil,
			0,
			&tmp[0],
			uint32(len(tmp)),
			&bytes,
			nil))

		if 0 == errc {
			errc = -fuse.EINVAL

			type SymbolicLinkReparseBuffer struct {
				ReparseTag           uint32
				ReparseDataLength    uint16
				Reserved             uint16
				SubstituteNameOffset uint16
				SubstituteNameLength uint16
				PrintNameOffset      uint16
				PrintNameLength      uint16
				Flags                uint32
				PathBuffer           [1]uint16
			}
			rbuf := (*SymbolicLinkReparseBuffer)(unsafe.Pointer(&tmp[0]))

			if uint32(unsafe.Sizeof(SymbolicLinkReparseBuffer{})) <= bytes &&
				syscall.IO_REPARSE_TAG_SYMLINK == rbuf.ReparseTag &&
				0 != rbuf.Flags&1 /*SYMLINK_FLAG_RELATIVE*/ {

				i, j := rbuf.SubstituteNameOffset/2, (rbuf.SubstituteNameOffset+rbuf.SubstituteNameLength)/2
				pbuf := (*[1 << 30]uint16)(unsafe.Pointer(&rbuf.PathBuffer))[i:j]
				path := string(utf16.Decode(pbuf))

				errc = 0
				target = strings.ReplaceAll(path, `\`, `/`)
			}
		}

		close(fh)
	}

	return
}

func Rename(oldpath string, newpath string) (errc int) {
	errc, fh := open(oldpath, 0x00010100 /*DELETE | FILE_WRITE_ATTRIBUTES*/, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		type FILE_RENAME_INFO struct {
			Flags          uint32
			RootDirectory  syscall.Handle
			FileNameLength uint32
			FileName       [1]uint16
		}

		nu16 := utf16.Encode([]rune(newpath))
		size := int(unsafe.Offsetof(FILE_RENAME_INFO{}.FileName)) + (len(nu16)+1)*2
		buf := make([]uint8, size)

		info := (*FILE_RENAME_INFO)(unsafe.Pointer(&buf[0]))
		info.Flags = 0x43 /*REPLACE_IF_EXISTS | POSIX_SEMANTICS | IGNORE_READONLY_ATTRIBUTE*/
		info.FileNameLength = uint32(len(nu16) * 2)

		nbuf := (*[1 << 30]uint16)(unsafe.Pointer(&info.FileName))[:len(nu16)+1]
		copy(nbuf, nu16)
		nbuf[len(nu16)] = 0

		r1, _, e := syscall.Syscall6(
			setFileInformationByHandle.Addr(),
			4,
			uintptr(fh),
			22, /*FileRenameInfoEx*/
			uintptr(unsafe.Pointer(info)),
			uintptr(size),
			0,
			0)
		if 0 == r1 && 87 /*ERROR_INVALID_PARAMETER*/ == e {
			info.Flags = 1 /*ReplaceIfExists*/
			r1, _, e = syscall.Syscall6(
				setFileInformationByHandle.Addr(),
				4,
				uintptr(fh),
				3, /*FileRenameInfo*/
				uintptr(unsafe.Pointer(info)),
				uintptr(size),
				0,
				0)
		}
		if 0 == r1 {
			errc = Errno(e)
		}

		close(fh)
	}

	return
}

func Chmod(path string, mode uint32) (errc int) {
	errc, fh := open(path, syscall.FILE_WRITE_ATTRIBUTES, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		errc = Fchmod(fh, mode)
		close(fh)
	}

	return
}

func Fchmod(fh uint64, mode uint32) (errc int) {
	info := _FILE_BASIC_INFO{}
	errc = getBasicInfo(fh, &info)
	if 0 == errc {
		attr := ^uint32(0x2000) /*FILE_ATTRIBUTE_NOT_CONTENT_INDEXED*/ & info.FileAttributes
		info = _FILE_BASIC_INFO{}
		info.FileAttributes = mapModeToFileAttributes(mode, attr)

		errc = setBasicInfo(fh, &info)
	}

	return
}

func Lchown(path string, uid int, gid int) (errc int) {
	return -fuse.ENOSYS
}

func Lchflags(path string, flags uint32) (errc int) {
	errc, fh := open(path, syscall.FILE_WRITE_ATTRIBUTES, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		info := _FILE_BASIC_INFO{}
		errc = getBasicInfo(fh, &info)
		if 0 == errc {
			attr := 0x2000 /*FILE_ATTRIBUTE_NOT_CONTENT_INDEXED*/ & info.FileAttributes
			info = _FILE_BASIC_INFO{}
			info.FileAttributes = mapFlagsToFileAttributes(flags, attr)

			errc = setBasicInfo(fh, &info)
		}

		close(fh)
	}

	return
}

func UtimesNano(path string, tmsp []fuse.Timespec) (errc int) {
	errc, fh := open(path, syscall.FILE_WRITE_ATTRIBUTES, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		info := _FILE_BASIC_INFO{}
		zero := fuse.Timespec{}
		if zero != tmsp[0] {
			copyFileTimeU64FromFuseTimespec(&info.LastAccessTime, tmsp[0])
		}
		if zero != tmsp[1] {
			copyFileTimeU64FromFuseTimespec(&info.LastWriteTime, tmsp[1])
		}
		if 3 <= len(tmsp) && zero != tmsp[2] {
			copyFileTimeU64FromFuseTimespec(&info.ChangeTime, tmsp[3])
		}
		if 4 <= len(tmsp) && zero != tmsp[3] {
			copyFileTimeU64FromFuseTimespec(&info.CreationTime, tmsp[3])
		}

		errc = setBasicInfo(fh, &info)

		close(fh)
	}

	return
}

func Open(path string, flags int, mode uint32) (errc int, fh uint64) {
	DesiredAccess := uint32(0)
	switch flags & (fuse.O_RDONLY | fuse.O_WRONLY | fuse.O_RDWR) {
	case fuse.O_RDONLY:
		DesiredAccess = syscall.GENERIC_READ
	case fuse.O_WRONLY:
		DesiredAccess = syscall.GENERIC_WRITE
	case fuse.O_RDWR:
		DesiredAccess = syscall.GENERIC_READ | syscall.GENERIC_WRITE
	}

	/* we recognize only the O_CREAT flag */
	CreateDisposition := uint32(syscall.OPEN_EXISTING)
	FileAttributes := uint32(0)
	switch flags & (fuse.O_CREAT) {
	case fuse.O_CREAT:
		CreateDisposition = syscall.CREATE_NEW
		FileAttributes = mapModeToFileAttributes(mode, 0)
	}

	errc, fh = open(path, DesiredAccess, CreateDisposition, FileAttributes)
	if 0 == errc && 0 != 0x2000&FileAttributes /*FILE_ATTRIBUTE_NOT_CONTENT_INDEXED*/ {
		/* FILE_ATTRIBUTE_NOT_CONTENT_INDEXED cannot be set by CreateFile; hence this malarkey */
		Fchmod(fh, mode)
	}

	return
}

func Lstat(path string, stat *fuse.Stat_t) (errc int) {
	errc, fh := open(path, 0x80 /*FILE_READ_ATTRIBUTES*/, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		errc = Fstat(fh, stat)
		close(fh)
	}

	return
}

func Fstat(fh uint64, stat *fuse.Stat_t) (errc int) {
	var info syscall.ByHandleFileInformation
	e := syscall.GetFileInformationByHandle(syscall.Handle(fh), &info)
	if nil != e {
		return Errno(e)
	}

	*stat = fuse.Stat_t{}
	stat.Mode = mapFileAttributesToMode(info.FileAttributes)
	stat.Nlink = 1
	stat.Size = (int64(info.FileSizeHigh) << 32) | int64(info.FileSizeLow)
	copyFuseTimespecFromFileTime(&stat.Birthtim, &info.CreationTime)
	copyFuseTimespecFromFileTime(&stat.Atim, &info.LastAccessTime)
	copyFuseTimespecFromFileTime(&stat.Mtim, &info.LastWriteTime)
	copyFuseTimespecFromFileTime(&stat.Ctim, &info.LastWriteTime)
	stat.Flags = mapFileAttributesToFlags(info.FileAttributes)

	return 0
}

func Truncate(path string, length int64) (errc int) {
	errc, fh := open(path, 2 /*FILE_WRITE_DATA*/, syscall.OPEN_EXISTING, 0)
	if 0 == errc {
		errc = Ftruncate(fh, length)
		close(fh)
	}

	return
}

func Ftruncate(fh uint64, length int64) (errc int) {
	type FILE_END_OF_FILE_INFO struct {
		EndOfFile int64
	}

	info := FILE_END_OF_FILE_INFO{}
	info.EndOfFile = length

	r1, _, e := syscall.Syscall6(
		setFileInformationByHandle.Addr(),
		4,
		uintptr(fh),
		6, /*FileEndOfFileInfo*/
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
		0,
		0)
	if 0 == r1 {
		return Errno(e)
	}

	return 0
}

func Pread(fh uint64, p []byte, offset int64) (n int) {
	var overlapped = syscall.Overlapped{
		Offset:     uint32(offset),
		OffsetHigh: uint32(offset >> 32),
	}
	var bytes uint32
	e := syscall.ReadFile(
		syscall.Handle(fh),
		p,
		&bytes,
		&overlapped)
	if nil != e {
		if syscall.ERROR_HANDLE_EOF == e {
			return 0
		}
		return Errno(e)
	}

	return int(bytes)
}

func Pwrite(fh uint64, p []byte, offset int64) (n int) {
	var overlapped = syscall.Overlapped{
		Offset:     uint32(offset),
		OffsetHigh: uint32(offset >> 32),
	}
	var bytes uint32
	e := syscall.WriteFile(
		syscall.Handle(fh),
		p,
		&bytes,
		&overlapped)
	if nil != e {
		return Errno(e)
	}

	return int(bytes)
}

func Close(fh uint64) (errc int) {
	return close(fh)
}

func Fsync(fh uint64) (errc int) {
	return Errno(syscall.FlushFileBuffers(syscall.Handle(fh)))
}

func Opendir(path string) (errc int, fh uint64) {
	return open(path, 1 /*FILE_LIST_DIRECTORY*/, syscall.OPEN_EXISTING, 0)
}

func Readdir(fh uint64, fill func(name string, stat *fuse.Stat_t, ofst int64) bool) (errc int) {
	type FILE_FULL_DIR_INFO struct {
		NextEntryOffset uint32
		FileIndex       uint32
		CreationTime    uint64
		LastAccessTime  uint64
		LastWriteTime   uint64
		ChangeTime      uint64
		EndOfFile       uint64
		AllocationSize  uint64
		FileAttributes  uint32
		FileNameLength  uint32
		EaSize          uint32
		FileName        [1]uint16
	}
	buf := [16 * 1024]uint8{}

	for {
		r1, _, e := syscall.Syscall6(
			getFileInformationByHandleEx.Addr(),
			4,
			uintptr(fh),                      /* FileHandle */
			14,                               /* FileInformationClass = FileFullDirectoryInfo */
			uintptr(unsafe.Pointer(&buf[0])), /* FileInformation */
			uintptr(len(buf)),                /* Length */
			0,
			0)
		if 0 == r1 {
			if syscall.ERROR_FILE_NOT_FOUND == e || syscall.ERROR_NO_MORE_FILES == e {
				return 0
			}
			return Errno(e)
		}

		for next := uint32(0); ; {
			info := (*FILE_FULL_DIR_INFO)(unsafe.Pointer(&buf[next]))
			next += info.NextEntryOffset

			nbuf := (*[1 << 30]uint16)(unsafe.Pointer(&info.FileName))[:info.FileNameLength/2]
			name := string(utf16.Decode(nbuf))

			stat := fuse.Stat_t{}
			stat.Mode = mapFileAttributesToMode(info.FileAttributes)
			stat.Nlink = 1
			stat.Size = int64(info.EndOfFile)
			copyFuseTimespecFromFileTimeU64(&stat.Birthtim, info.CreationTime)
			copyFuseTimespecFromFileTimeU64(&stat.Atim, info.LastAccessTime)
			copyFuseTimespecFromFileTimeU64(&stat.Mtim, info.LastWriteTime)
			copyFuseTimespecFromFileTimeU64(&stat.Ctim, info.LastWriteTime)
			stat.Flags = mapFileAttributesToFlags(info.FileAttributes)

			if !fill(name, &stat, 0) {
				return 0
			}

			if 0 == info.NextEntryOffset {
				break
			}
		}
	}
}

func Closedir(fh uint64) (errc int) {
	return close(fh)
}

func Umask(mask int) (oldmask int) {
	return -fuse.ENOSYS
}

func Errno(err error) int {
	if nil == err {
		return 0
	}

	if e, ok := err.(syscall.Errno); ok {
		switch e {
		case 1 /*ERROR_INVALID_FUNCTION*/ :
			return -fuse.EINVAL
		case 2 /*ERROR_FILE_NOT_FOUND*/ :
			return -fuse.ENOENT
		case 3 /*ERROR_PATH_NOT_FOUND*/ :
			return -fuse.ENOENT
		case 4 /*ERROR_TOO_MANY_OPEN_FILES*/ :
			return -fuse.EMFILE
		case 5 /*ERROR_ACCESS_DENIED*/ :
			return -fuse.EACCES
		case 6 /*ERROR_INVALID_HANDLE*/ :
			return -fuse.EBADF
		case 7 /*ERROR_ARENA_TRASHED*/ :
			return -fuse.ENOMEM
		case 8 /*ERROR_NOT_ENOUGH_MEMORY*/ :
			return -fuse.ENOMEM
		case 9 /*ERROR_INVALID_BLOCK*/ :
			return -fuse.ENOMEM
		case 10 /*ERROR_BAD_ENVIRONMENT*/ :
			return -fuse.E2BIG
		case 11 /*ERROR_BAD_FORMAT*/ :
			return -fuse.ENOEXEC
		case 12 /*ERROR_INVALID_ACCESS*/ :
			return -fuse.EINVAL
		case 13 /*ERROR_INVALID_DATA*/ :
			return -fuse.EINVAL
		case 15 /*ERROR_INVALID_DRIVE*/ :
			return -fuse.ENOENT
		case 16 /*ERROR_CURRENT_DIRECTORY*/ :
			return -fuse.EACCES
		case 17 /*ERROR_NOT_SAME_DEVICE*/ :
			return -fuse.EXDEV
		case 18 /*ERROR_NO_MORE_FILES*/ :
			return -fuse.ENOENT
		case 33 /*ERROR_LOCK_VIOLATION*/ :
			return -fuse.EACCES
		case 53 /*ERROR_BAD_NETPATH*/ :
			return -fuse.ENOENT
		case 65 /*ERROR_NETWORK_ACCESS_DENIED*/ :
			return -fuse.EACCES
		case 67 /*ERROR_BAD_NET_NAME*/ :
			return -fuse.ENOENT
		case 80 /*ERROR_FILE_EXISTS*/ :
			return -fuse.EEXIST
		case 82 /*ERROR_CANNOT_MAKE*/ :
			return -fuse.EACCES
		case 83 /*ERROR_FAIL_I24*/ :
			return -fuse.EACCES
		case 87 /*ERROR_INVALID_PARAMETER*/ :
			return -fuse.EINVAL
		case 89 /*ERROR_NO_PROC_SLOTS*/ :
			return -fuse.EAGAIN
		case 108 /*ERROR_DRIVE_LOCKED*/ :
			return -fuse.EACCES
		case 109 /*ERROR_BROKEN_PIPE*/ :
			return -fuse.EPIPE
		case 112 /*ERROR_DISK_FULL*/ :
			return -fuse.ENOSPC
		case 114 /*ERROR_INVALID_TARGET_HANDLE*/ :
			return -fuse.EBADF
		case 128 /*ERROR_WAIT_NO_CHILDREN*/ :
			return -fuse.ECHILD
		case 129 /*ERROR_CHILD_NOT_COMPLETE*/ :
			return -fuse.ECHILD
		case 130 /*ERROR_DIRECT_ACCESS_HANDLE*/ :
			return -fuse.EBADF
		case 131 /*ERROR_NEGATIVE_SEEK*/ :
			return -fuse.EINVAL
		case 132 /*ERROR_SEEK_ON_DEVICE*/ :
			return -fuse.EACCES
		case 145 /*ERROR_DIR_NOT_EMPTY*/ :
			return -fuse.ENOTEMPTY
		case 158 /*ERROR_NOT_LOCKED*/ :
			return -fuse.EACCES
		case 161 /*ERROR_BAD_PATHNAME*/ :
			return -fuse.ENOENT
		case 164 /*ERROR_MAX_THRDS_REACHED*/ :
			return -fuse.EAGAIN
		case 167 /*ERROR_LOCK_FAILED*/ :
			return -fuse.EACCES
		case 183 /*ERROR_ALREADY_EXISTS*/ :
			return -fuse.EEXIST
		case 206 /*ERROR_FILENAME_EXCED_RANGE*/ :
			return -fuse.ENOENT
		case 215 /*ERROR_NESTING_NOT_ALLOWED*/ :
			return -fuse.EAGAIN
		case 1816 /*ERROR_NOT_ENOUGH_QUOTA*/ :
			return -fuse.ENOMEM
		case 4390 /*ERROR_NOT_A_REPARSE_POINT*/ :
			return -fuse.EINVAL
		default:
			if 19 /*ERROR_WRITE_PROTECT*/ <= e && e <= 36 /*ERROR_SHARING_BUFFER_EXCEEDED*/ {
				return -fuse.EACCES
			} else if 188 /*ERROR_INVALID_STARTING_CODESEG*/ <= e && e <= 202 /*ERROR_INFLOOP_IN_RELOC_CHAIN*/ {
				return -fuse.ENOEXEC
			} else {
				return -fuse.EINVAL
			}
		}
	}

	return -fuse.EINVAL
}

func open(
	path string, DesiredAccess uint32, CreateDisposition uint32, FlagsAndAttributes uint32) (
	errc int, fh uint64) {
	openReparsePoint := uint32(syscall.FILE_FLAG_OPEN_REPARSE_POINT)
	if strings.HasSuffix(path, `\`) {
		openReparsePoint = 0
	}

	pathu16, e := syscall.UTF16PtrFromString(path)
	if nil != e {
		return Errno(e), ^uint64(0)
	}

	h, e := syscall.CreateFile(
		pathu16,
		DesiredAccess,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		CreateDisposition,
		FlagsAndAttributes|openReparsePoint|syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0)
	if nil != e {
		return Errno(e), ^uint64(0)
	}

	return 0, uint64(h)
}

func close(fh uint64) (errc int) {
	return Errno(syscall.CloseHandle(syscall.Handle(fh)))
}

func getBasicInfo(fh uint64, info *_FILE_BASIC_INFO) (errc int) {
	r1, _, e := syscall.Syscall6(
		getFileInformationByHandleEx.Addr(),
		4,
		uintptr(fh),
		0, /*FileBasicInfo*/
		uintptr(unsafe.Pointer(info)),
		unsafe.Sizeof(*info),
		0,
		0)
	if 0 == r1 {
		return Errno(e)
	}

	return 0
}

func setBasicInfo(fh uint64, info *_FILE_BASIC_INFO) (errc int) {
	r1, _, e := syscall.Syscall6(
		setFileInformationByHandle.Addr(),
		4,
		uintptr(fh),
		0, /*FileBasicInfo*/
		uintptr(unsafe.Pointer(info)),
		unsafe.Sizeof(*info),
		0,
		0)
	if 0 == r1 {
		return Errno(e)
	}

	return 0
}

func copyFuseTimespecFromFileTime(dst *fuse.Timespec, src *syscall.Filetime) {
	dst.Nsec = src.Nanoseconds()
	dst.Sec, dst.Nsec = dst.Nsec/1000000000, dst.Nsec%1000000000
}

func copyFuseTimespecFromFileTimeU64(dst *fuse.Timespec, srcU64 uint64) {
	src := syscall.Filetime{
		LowDateTime:  uint32(srcU64),
		HighDateTime: uint32(srcU64 >> 32),
	}
	dst.Nsec = src.Nanoseconds()
	dst.Sec, dst.Nsec = dst.Nsec/1000000000, dst.Nsec%1000000000
}

func copyFileTimeU64FromFuseTimespec(dstU64 *uint64, src fuse.Timespec) {
	dst := syscall.NsecToFiletime(src.Sec*1000000000 + src.Nsec)
	*dstU64 = (uint64(dst.HighDateTime) << 32) | uint64(dst.LowDateTime)
}

func mapFileAttributesToMode(attr uint32) (mode uint32) {
	if 0 != attr&syscall.FILE_ATTRIBUTE_REPARSE_POINT {
		mode = 0777 | fuse.S_IFLNK
	} else if 0 != attr&syscall.FILE_ATTRIBUTE_DIRECTORY {
		mode = 0777 | fuse.S_IFDIR
	} else {
		mode = 0666 | fuse.S_IFREG
		if 0 == attr&0x2000 /*FILE_ATTRIBUTE_NOT_CONTENT_INDEXED*/ {
			/* abuse FILE_ATTRIBUTE_NOT_CONTENT_INDEXED to store the NOT executable condition */
			mode |= 0111
		}
	}

	return
}

func mapModeToFileAttributes(mode uint32, extra uint32) (attr uint32) {
	attr = extra
	if 0 == mode&0100 {
		/* abuse FILE_ATTRIBUTE_NOT_CONTENT_INDEXED to store the NOT executable condition */
		attr |= 0x2000 /*FILE_ATTRIBUTE_NOT_CONTENT_INDEXED*/
	}
	if 0 == attr {
		attr = syscall.FILE_ATTRIBUTE_NORMAL
	}

	return
}

func mapFileAttributesToFlags(attr uint32) (flags uint32) {
	if 0 != attr&syscall.FILE_ATTRIBUTE_ARCHIVE {
		flags |= fuse.UF_ARCHIVE
	}
	if 0 != attr&syscall.FILE_ATTRIBUTE_HIDDEN {
		flags |= fuse.UF_HIDDEN
	}
	if 0 != attr&syscall.FILE_ATTRIBUTE_READONLY {
		flags |= fuse.UF_READONLY
	}
	if 0 != attr&syscall.FILE_ATTRIBUTE_SYSTEM {
		flags |= fuse.UF_SYSTEM
	}

	return
}

func mapFlagsToFileAttributes(flags uint32, extra uint32) (attr uint32) {
	attr = extra
	if 0 != flags&fuse.UF_ARCHIVE {
		attr |= syscall.FILE_ATTRIBUTE_ARCHIVE
	}
	if 0 != flags&fuse.UF_HIDDEN {
		attr |= syscall.FILE_ATTRIBUTE_HIDDEN
	}
	if 0 != flags&fuse.UF_READONLY {
		attr |= syscall.FILE_ATTRIBUTE_READONLY
	}
	if 0 != flags&fuse.UF_SYSTEM {
		attr |= syscall.FILE_ATTRIBUTE_SYSTEM
	}
	if 0 == attr {
		attr = syscall.FILE_ATTRIBUTE_NORMAL
	}

	return
}

func Setuidgid() func() {
	return func() {
	}
}

type _FILE_BASIC_INFO struct {
	CreationTime   uint64
	LastAccessTime uint64
	LastWriteTime  uint64
	ChangeTime     uint64
	FileAttributes uint32
}

var (
	kernel32                     = syscall.MustLoadDLL("kernel32.dll")
	getDiskFreeSpaceW            = kernel32.MustFindProc("GetDiskFreeSpaceW")
	getFileInformationByHandleEx = kernel32.MustFindProc("GetFileInformationByHandleEx")
	getVolumeInformationW        = kernel32.MustFindProc("GetVolumeInformationW")
	getVolumePathNameW           = kernel32.MustFindProc("GetVolumePathNameW")
	setFileInformationByHandle   = kernel32.MustFindProc("SetFileInformationByHandle")
)
