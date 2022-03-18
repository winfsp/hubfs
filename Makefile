# Makefile

MyBuildNumber = $(shell date +%y%j)
MyVersion = 1.0.$(MyBuildNumber)
MyProductVersion = "2022 Beta2"
MyProductStage = "Beta"

MyProductName = "HUBFS"
MyDescription = "File system for GitHub"
MyCopyright = "2021-2022 Bill Zissimopoulos"
MyCompanyName = "Navimatics LLC"

CertIssuer = "DigiCert"
CrossCert = "DigiCert High Assurance EV Root CA.crt"

ifneq ($(OS),Windows_NT)
	OS=$(shell uname)
endif

ExeSuffix=
ifeq ($(OS),Windows_NT)
	GoBuild=go build
	ExeSuffix=.exe
endif
ifeq ($(OS),Linux)
	GoBuild=go build
	export CGO_CFLAGS=-include $(dir $(realpath $(lastword $(MAKEFILE_LIST))))ext/glibc-compat/glibc-2.17.h
endif
ifeq ($(OS),Darwin)
	GoBuild=../gobuild.mac
endif

.PHONY: default
default: build

.PHONY: build
build:
	cd src && \
	$(GoBuild) \
		-ldflags "-s -w \
			-X \"main.MyVersion=$(subst $\",,$(MyVersion))\" \
			-X \"main.MyProductVersion=$(subst $\",,$(MyProductVersion))\" \
			-X \"main.MyProductName=$(subst $\",,$(MyProductName))\" \
			-X \"main.MyDescription=$(subst $\",,$(MyDescription))\" \
			-X \"main.MyCopyright=$(subst $\",,$(MyCopyright))\" \
			" \
		-o ../hubfs$(ExeSuffix)

.PHONY: racy
racy:
	cd src && \
	go build -race -o ../hubfs$(ExeSuffix)

.PHONY: test
test:
	cd src && \
	go test -count=1 ./...

.PHONY: dist
dist: build
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -NonInteractive -ExecutionPolicy Unrestricted \
		"Compress-Archive -Force -Path hubfs.exe -DestinationPath .\hubfs-win-$(MyVersion).zip"
	candle -nologo -arch x64 -pedantic \
		-dMyVersion=$(MyVersion) \
		-dMyProductVersion=$(MyProductVersion) \
		-dMyProductStage=$(MyProductStage) \
		-dMyProductName=$(MyProductName) \
		-dMyDescription=$(MyDescription) \
		-dMyCompanyName=$(MyCompanyName) \
		-dMyArch=x64 \
		-o hubfs.wixobj \
		hubfs.wxs
	light -nologo \
		-ext WixUIExtension \
		-spdb \
		-o hubfs-win-$(MyVersion).msi \
		hubfs.wixobj
	powershell -NoProfile -NonInteractive -ExecutionPolicy Unrestricted \
		"Remove-Item -Force hubfs.wixobj"
	signtool sign \
		/ac $(CrossCert) \
		/i $(CertIssuer) \
		/n $(MyCompanyName) \
		/d $(MyDescription) \
		/fd sha1 \
		/t http://timestamp.digicert.com \
		hubfs-win-$(MyVersion).msi || \
		echo "SIGNING FAILED! The product has been successfully built, but not signed." 1>&2
endif
ifeq ($(OS),Linux)
	rm -f hubfs-lnx-$(MyVersion).zip
	zip hubfs-lnx-$(MyVersion).zip hubfs
endif
ifeq ($(OS),Darwin)
	rm -f hubfs-mac-$(MyVersion).zip
	zip hubfs-mac-$(MyVersion).zip hubfs
endif
