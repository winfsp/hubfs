# Makefile for Hubfs
#
# Copyright 2021-2022 Bill Zissimopoulos
#
# This file is part of Hubfs.
#
# You can redistribute it and/or modify it under the terms of the GNU
# Affero General Public License version 3 as published by the Free
# Software Foundation.

MyProductName = "HUBFS"
MyDescription = "File system for GitHub"
MyCopyright = "2021-2022 Bill Zissimopoulos"
MyCompanyName = "Navimatics LLC"
MyCertIssuer = "DigiCert"
MyCrossCert = "DigiCert High Assurance EV Root CA.crt"
MyBuildNumber = $(shell date +%y%j)
MyVersion = 1.0.$(MyBuildNumber)
MyProductVersion = "2022 Beta2"
MyProductStage = "Beta"

ifeq ($(TAG),)
MyProductTag = ""
else ifeq ($(TAG),pro)
MyProductTag = "Professional"
else ifeq ($(TAG),ent)
MyProductTag = "Enterprise"
else
$(error invalid TAG; possible options: <empty> pro ent)
endif

BldDir = $(dir $(realpath $(lastword $(MAKEFILE_LIST))))
SrcDir = $(BldDir)../src/
ExtDir = $(BldDir)../ext/
OutDir = $(BldDir)out/

ifneq ($(OS),Windows_NT)
OS = $(shell uname)
endif

GoBuild = go build
ExeSuffix =
ifeq ($(OS),Windows_NT)
ExeSuffix = .exe
else ifeq ($(OS),Linux)
export CGO_CFLAGS=-include $(ExtDir)glibc-compat/glibc-2.17.h
else ifeq ($(OS),Darwin)
GoBuild = $(BldDir)gobuild.mac
endif

GoBuildTags =
TagInfix =
ifdef TAG
GoBuildTags = -tags $(TAG)
TagInfix = -$(TAG)
endif

.PHONY: default
default: build

.PHONY: build
build:
	cd $(SrcDir) && \
	$(GoBuild) \
		$(GoBuildTags) \
		-buildvcs=false \
		-trimpath \
		-ldflags "-s -w \
			-X \"main.MyProductName=$(subst $\",,$(MyProductName))\" \
			-X \"main.MyDescription=$(subst $\",,$(MyDescription))\" \
			-X \"main.MyCopyright=$(subst $\",,$(MyCopyright))\" \
			-X \"main.MyVersion=$(subst $\",,$(MyVersion))\" \
			-X \"main.MyProductVersion=$(subst $\",,$(MyProductVersion))\" \
			-X \"main.MyProductTag=$(subst $\",,$(MyProductTag))\" \
			" \
		-o $(OutDir)hubfs$(ExeSuffix)

.PHONY: racy
racy:
	cd $(SrcDir) && \
	go build $(GoBuildTags) -race -o $(OutDir)hubfs$(ExeSuffix)

.PHONY: test
test:
	cd $(SrcDir) && \
	go test $(GoBuildTags) -count=1 ./...

.PHONY: dist
dist: build
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -NonInteractive -ExecutionPolicy Unrestricted \
		"Compress-Archive -Force -Path $(OutDir)hubfs.exe -DestinationPath $(OutDir)hubfs$(TagInfix)-win-$(MyVersion).zip"
	candle -nologo -arch x64 -pedantic \
		-dMyProductName=$(MyProductName) \
		-dMyProductTag=$(MyProductTag) \
		-dMyDescription=$(MyDescription) \
		-dMyCompanyName=$(MyCompanyName) \
		-dMyVersion=$(MyVersion) \
		-dMyProductVersion=$(MyProductVersion) \
		-dMyProductStage=$(MyProductStage) \
		-o $(OutDir)hubfs$(TagInfix).wixobj \
		$(BldDir)hubfs.wxs
	light -nologo \
		-ext WixUIExtension \
		-sice:ICE61 \
		-spdb \
		-o $(OutDir)hubfs$(TagInfix)-win-$(MyVersion).msi \
		$(OutDir)hubfs$(TagInfix).wixobj
	signtool sign \
		/ac $(MyCrossCert) \
		/i $(MyCertIssuer) \
		/n $(MyCompanyName) \
		/d $(MyDescription) \
		/fd sha1 \
		/t http://timestamp.digicert.com \
		$(OutDir)hubfs$(TagInfix)-win-$(MyVersion).msi || \
		echo "SIGNING FAILED! The product has been successfully built, but not signed." 1>&2
endif
ifeq ($(OS),Linux)
	rm -f $(OutDir)hubfs$(TagInfix)-lnx-$(MyVersion).zip
	zip $(OutDir)hubfs$(TagInfix)-lnx-$(MyVersion).zip $(OutDir)hubfs
endif
ifeq ($(OS),Darwin)
	rm -f $(OutDir)hubfs$(TagInfix)-mac-$(MyVersion).zip
	zip $(OutDir)hubfs$(TagInfix)-mac-$(MyVersion).zip $(OutDir)hubfs
endif
