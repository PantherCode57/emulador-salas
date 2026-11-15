package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"io"
	"math"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	xdraw "golang.org/x/image/draw"
)

// Fotos de perfil: se piden en tamaño pequeño («preview») solo para los chats que se ven en
// pantalla, se recortan en redondo y se quedan en memoria (no se guardan en la consola).

const ladoAvatar = 42

type avatares struct {
	a       *app
	imgs    map[string]image.Image // jid -> foto redonda; nil = no tiene (o no deja verla)
	pedidos map[string]bool
	turno   chan struct{}
}

func nuevosAvatares(a *app) *avatares {
	return &avatares{a: a, imgs: map[string]image.Image{}, pedidos: map[string]bool{}, turno: make(chan struct{}, 2)}
}

// de devuelve la foto de perfil de un chat si ya está; si no se ha pedido, la pide.
func (av *avatares) de(jid string) image.Image {
	if img, ok := av.imgs[jid]; ok {
		return img
	}
	if av.pedidos[jid] || av.a.w == nil || av.a.w.cli == nil || av.a.estado != waConectado {
		return nil
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		av.imgs[jid] = nil
		return nil
	}
	av.pedidos[jid] = true
	cli := av.a.w.cli
	otros := av.a.w.candidatos(j)
	go func() {
		av.turno <- struct{}{}
		defer func() { <-av.turno }()
		var img image.Image
		for _, c := range otros { // con el código LID y, si no, con el número
			if img = bajarAvatar(cli, c); img != nil {
				break
			}
		}
		av.a.hacer(func() { av.imgs[jid] = img })
	}()
	return nil
}

func bajarAvatar(cli *whatsmeow.Client, jid types.JID) image.Image {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := cli.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
	if err != nil || info == nil || info.URL == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	src, _, err := image.Decode(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil
	}
	return redonda(src, ladoAvatar)
}

// redonda recorta la imagen en un círculo de 'lado' píxeles (con el borde suavizado).
func redonda(src image.Image, lado int) image.Image {
	b := src.Bounds()
	cuadro := min(b.Dx(), b.Dy())
	r := image.Rect(b.Min.X+(b.Dx()-cuadro)/2, b.Min.Y+(b.Dy()-cuadro)/2, 0, 0)
	r.Max = r.Min.Add(image.Pt(cuadro, cuadro))
	esc := image.NewRGBA(image.Rect(0, 0, lado, lado))
	xdraw.CatmullRom.Scale(esc, esc.Bounds(), src, r, draw.Src, nil)
	mascara := image.NewAlpha(esc.Bounds())
	c := float64(lado) / 2
	for y := 0; y < lado; y++ {
		for x := 0; x < lado; x++ {
			dx, dy := float64(x)+0.5-c, float64(y)+0.5-c
			d := c - math.Sqrt(dx*dx+dy*dy)
			a := min(max(d+0.5, 0), 1)
			mascara.SetAlpha(x, y, color.Alpha{A: uint8(a * 255)})
		}
	}
	out := image.NewRGBA(esc.Bounds())
	draw.DrawMask(out, out.Bounds(), esc, image.Point{}, mascara, image.Point{}, draw.Over)
	return out
}
