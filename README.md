# HUBFS - File System for GitHub

HUBFS is a read-only file system for GitHub and Git. Git repositories and their contents are represented as regular directories and files and are accessible by any application, without the application having any knowledge that it is really accessing a remote Git repository.

<img src="doc/cap1.gif" width="75%"/>

The above screen capture shows HUBFS on Windows accessed from Powershell and VSCode.

## Installation

**Windows**

- Install the latest version of [WinFsp](https://github.com/billziss-gh/winfsp/releases).
- Install the latest version of [HUBFS](https://github.com/billziss-gh/hubfs/releases). The Windows installer (MSI) includes better integration with Windows and is the recommended installation method. There is also a standalone Windows executable (ZIP) if you prefer.

**macOS**

**Linux**

## How to use

HUBFS is a command-line program with a usage documented below. On Windows there is also better integration with the system so that you can use HUBFS without the command line.

HUBFS supports both authenticated and non-authenticated access to repositories. When using HUBFS without authentication, only public repositories are available. When using HUBFS with authentication, both public and private repositories become available; an additional benefit is that the rate limiting that GitHub does for certain operations is relaxed.

In order to mount HUBFS issue the command `hubfs MOUNTPOINT`. For example, `hubfs H:` on Windows or `hubfs mnt` on macOS and Linux.

The first time you run HUBFS you will be prompted to authorize with GitHub:

```
> ./hubfs H:
First, copy your one-time code: XXXX-XXXX
Then press [Enter] to continue in the web browser...
```

HUBFS will then open your system browser where you will be able to authorize it with GitHub. HUBFS will store the resulting authorization token in the system keyring (Windows Credential Manager, macOS Keychain, etc.). Subsequent runs of HUBFS will use the authorization token from the system keyring and you will not be required to re-authorize the application.

To unmount the file system simply use <kbd>Ctl-C</kbd>. On macOS and Linux you may also be able to unmount using `umount` or `fusermount -u`.

### Windows integration

When you use the MSI installer under Windows there is better integration of HUBFS with the rest of the system:

- There is a "Start Menu > HUBFS > Perform GitHub auth" shortcut that allows you to authorize HUBFS with GitHub without using the command line.

- You can mount HUBFS drives using the Windows Explorer "Map Network Drive" functionality. To dismount use the "Disconnect Network Drive" functionality. (It is recommended to first authorize HUBFS with GitHub using the above mentioned shortcut.)

    <img src="doc/mapnet.png" width="50%"/>

- You can also mount HUBFS with the `net use` command. The command `net use H: \\hubfs\github.com` will mount HUBFS as drive `H:`. The command `net use H: /delete` will dismount the `H:` drive.

## How to build

In order to build run `make` from the project's root directory. On Windows you will have to run `.\make`. The build prerequisites for individual platforms are listed below:

- Windows: [Go 1.16](https://golang.org/dl/), [WinFsp](https://github.com/billziss-gh/winfsp), gcc (e.g. from [Mingw-builds](http://mingw-w64.org/doku.php/download))

- macOS: [Go 1.16](https://golang.org/dl/), [FUSE for macOS](https://osxfuse.github.io), [command line tools](https://developer.apple.com/library/content/technotes/tn2339/_index.html)

- Linux: Prerequisites: [Go 1.16](https://golang.org/dl/), libfuse-dev, gcc

## How it works

HUBFS is a cross-platform file system written in Go.

## Known Issues

## Potential Future Improvements
