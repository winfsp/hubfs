# Makefile

MyProductName = "hubfs"
MyDescription = "File system for GitHub"
MyCompanyName = "Navimatics LLC"
MyCopyright = "2021 Bill Zissimopoulos"
MyProductVersion = "2021 Beta1"
MyBuildNumber = $(shell date +%y%j)
MyVersion = 0.1.$(MyBuildNumber)
MyRepository = "https://github.com/billziss-gh/hubfs"

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
	go build -ldflags "-s -w -X \"main.MyVersion=$(MyVersion)\"" -o hubfs$(ExeSuffix)

.PHONY: racy
racy:
	go build -race -o hubfs$(ExeSuffix)

.PHONY: msi
msi: build
	$(WIX)\bin\candle -nologo -arch x64 -pedantic\
		-dMyProductName=$(MyProductName)\
		-dMyCompanyName=$(MyCompanyName)\
		-dMyDescription=$(MyDescription)\
		-dMyProductVersion=$(MyProductVersion)\
		-dMyVersion=$(MyVersion)\
		-dMyArch=x64\
		-o hubfs.wixobj\
		hubfs.wxs
	$(WIX)\bin\light -nologo\
		-o hubfs-win-$(MyVersion).msi\
		-ext WixUIExtension\
		hubfs.wixobj
