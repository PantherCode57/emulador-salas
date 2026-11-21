//go:build !linux

package main

import "errors"

func reabrir(yo string) error { return errors.New("solo en la consola") }
