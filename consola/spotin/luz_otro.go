//go:build !linux

package main

type luz struct{ apagada bool }

func nuevaLuz() *luz    { return &luz{} }
func (l *luz) apagar()   { l.apagada = true }
func (l *luz) encender() { l.apagada = false }
