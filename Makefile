# Makefile

MyBuildNumber = $(shell date +%y%j)
MyVersion = 0.1.$(MyBuildNumber)
MyProductVersion = "2021 Beta1"

MyProductName = "HUBFS"
MyDescription = "File system for GitHub"
MyCopyright = "2021 Bill Zissimopoulos"
MyCompanyName = "Navimatics LLC"

CertIssuer = "DigiCert"
CrossCert = "DigiCert High Assurance EV Root CA.crt"

ifeq ($(OS),Windows_NT)
ExeSuffix=.exe
else
ExeSuffix=
endif

.PHONY: default
default: build

.PHONY: build
build:
	go build \
		-ldflags "-s -w \
			-X \"main.MyVersion=$(subst $\",,$(MyVersion))\" \
			-X \"main.MyProductVersion=$(subst $\",,$(MyProductVersion))\" \
			-X \"main.MyProductName=$(subst $\",,$(MyProductName))\" \
			-X \"main.MyDescription=$(subst $\",,$(MyDescription))\" \
			-X \"main.MyCopyright=$(subst $\",,$(MyCopyright))\" \
			" \
		-o hubfs$(ExeSuffix)

.PHONY: racy
racy:
	go build -race -o hubfs$(ExeSuffix)

.PHONY: msi
msi: build
	candle -nologo -arch x64 -pedantic \
		-dMyVersion=$(MyVersion) \
		-dMyProductVersion=$(MyProductVersion) \
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
	del hubfs.wixobj
	signtool sign \
		/ac $(CrossCert) \
		/i $(CertIssuer) \
		/n $(MyCompanyName) \
		/d $(MyDescription) \
		/fd sha1 \
		/t http://timestamp.digicert.com \
		hubfs-win-$(MyVersion).msi || \
		echo "SIGNING FAILED! The product has been successfully built, but not signed." 1>&2
