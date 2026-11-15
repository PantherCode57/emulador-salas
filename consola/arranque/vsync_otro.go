//go:build !linux

package main

func esperarVsync(fd uintptr) {}
