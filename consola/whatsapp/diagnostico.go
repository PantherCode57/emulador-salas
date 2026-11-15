package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// diagnostico pide un QR sin pantalla y con todo el registro de whatsmeow, en una base de datos
// aparte (no toca la sesión de verdad). Sirve para ver por qué no llega el código.
func diagnostico(cuanto time.Duration) {
	_ = os.MkdirAll(carpetaDatos, 0o700)
	ruta := filepath.Join(carpetaDatos, "diagnostico.db")
	_ = os.Remove(ruta)
	db, err := sql.Open("sqlite", "file:"+ruta+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Println("bd:", err)
		return
	}
	c := sqlstore.NewWithDB(db, "sqlite3", waLog.Stdout("BD", "INFO", false))
	if err := c.Upgrade(context.Background()); err != nil {
		fmt.Println("bd:", err)
		return
	}
	cli := whatsmeow.NewClient(c.NewDevice(), waLog.Stdout("WA", "DEBUG", false))
	cli.AddEventHandler(func(evt any) { fmt.Printf("EVENTO %T\n", evt) })
	qr, err := cli.GetQRChannel(context.Background())
	if err != nil {
		fmt.Println("QR:", err)
		return
	}
	inicio := time.Now()
	fmt.Println("conectando…")
	if err := cli.Connect(); err != nil {
		fmt.Println("conectar:", err)
		return
	}
	fmt.Printf("conectado en %v\n", time.Since(inicio).Round(time.Millisecond))
	fin := time.After(cuanto)
	for {
		select {
		case e, ok := <-qr:
			if !ok {
				fmt.Println("el canal del QR se ha cerrado")
				cli.Disconnect()
				return
			}
			if e.Event == "code" {
				fmt.Printf("QR RECIBIDO a los %v (%d caracteres)\n", time.Since(inicio).Round(time.Millisecond), len(e.Code))
			} else {
				fmt.Printf("QR: %s %v\n", e.Event, e.Error)
			}
		case <-fin:
			cli.Disconnect()
			_ = os.Remove(ruta)
			return
		}
	}
}
