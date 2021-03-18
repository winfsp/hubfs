# Makefile

MyBuildNumber=$(shell date +%y%j)
MyVersion=0.1.$(MyBuildNumber)

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
