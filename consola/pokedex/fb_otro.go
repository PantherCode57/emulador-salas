//go:build !linux

package main

import "image"

func pintarFB(img *image.RGBA) {}
