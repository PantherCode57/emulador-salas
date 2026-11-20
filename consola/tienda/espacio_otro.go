//go:build !linux

package main

func espacioLibre(dir string) int64 { return -1 }
