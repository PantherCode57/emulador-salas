package main

import (
	"image"
	"image/draw"
	"math"
	"math/rand"

	xdraw "golang.org/x/image/draw"
)

// La carrera: Zarpa corre sola hacia delante por tres carriles (−1, 0 y 1). Izquierda y derecha
// cambian de carril, arriba (o A) salta, abajo (o B) se agacha. Las vallas se saltan, los carteles
// de la autopista se pasan agachada y los camiones de tacos hay que esquivarlos. Las estrellas se
// cogen; el imán las atrae y el escudo aguanta un golpe. Cada vez va más rápido.

type tipoObj int

const (
	oValla tipoObj = iota
	oCartel
	oCamion
	oEstrella
	oIman
	oEscudo
	oPalmera
)

type objeto struct {
	tipo   tipoObj
	carril int     // −1, 0 o 1 (las palmeras: −1 izquierda, 1 derecha)
	s      float64 // dónde está, en el recorrido
	alto   float64 // las estrellas: a qué altura
	cogido bool
	vuela  float64 // las estrellas atraídas por el imán: lo que llevan de camino (0..1)
}

const (
	velInicial   = 12.0
	velMaxima    = 28.0
	gravedad     = 30.0
	impulsoSalto = 7.9 // sube ~1 carril de alto: pasa las vallas y se queda en la pantalla
	tAgachado    = 0.62
	altoZarpaU   = 0.66 // lo que mide Zarpa de pie (agachada, la mitad)
	tIman        = 9.0
	tEscudo      = 12.0
)

type carrera struct {
	rng         *rand.Rand
	recorrido   float64
	vel         float64
	tiempo      float64
	carril      int
	carrilX     float64
	y, vy       float64
	agachado    float64
	objs        []*objeto
	generado    float64
	palmeras    float64
	ultimoPoder float64
	estrellas   int
	iman        float64
	escudo      float64
	viva        bool
	muerta      float64 // tiempo desde el choque
	fase        float64 // de la carrera (para dibujar las piernas)
	temblor     float64
	avisos      []string // lo que ha pasado en este paso (para los sonidos): «salto», «estrella»…
	soloAdorno  bool     // la del título: solo palmeras y estrellas
}

func nuevaCarrera(semilla int64) *carrera {
	c := &carrera{rng: rand.New(rand.NewSource(semilla)), vel: velInicial, viva: true, generado: 14, palmeras: -4}
	return c
}

func (c *carrera) metros() int { return int(c.recorrido) }

func (c *carrera) puntos() int { return c.metros() + c.estrellas*10 }

// ---- Lo que haces -------------------------------------------------------------------------------------

func (c *carrera) mover(d int) {
	if !c.viva {
		return
	}
	n := min(max(c.carril+d, -1), 1)
	if n != c.carril {
		c.carril = n
		c.avisos = append(c.avisos, "carril")
	}
}

func (c *carrera) saltar() {
	if !c.viva || c.y > 0.02 {
		return
	}
	c.vy, c.agachado = impulsoSalto, 0
	c.avisos = append(c.avisos, "salto")
}

func (c *carrera) agacharse() {
	if !c.viva {
		return
	}
	if c.y > 0.02 { // en el aire: baja de golpe
		c.vy = -impulsoSalto * 1.4
	}
	c.agachado = tAgachado
	c.avisos = append(c.avisos, "agacharse")
}

// ---- Cada paso --------------------------------------------------------------------------------------

func (c *carrera) paso(dt float64) {
	c.avisos = c.avisos[:0]
	if !c.viva {
		c.muerta += dt
		c.vel = math.Max(0, c.vel-40*dt)
		c.recorrido += c.vel * dt
		c.temblor = math.Max(0, c.temblor-dt*3)
		return
	}
	c.tiempo += dt
	c.vel = math.Min(velMaxima, velInicial+c.tiempo*0.16)
	c.recorrido += c.vel * dt
	c.fase += dt * (8 + c.vel*0.35)
	// cambiar de carril, suave
	obj := float64(c.carril)
	if d := obj - c.carrilX; math.Abs(d) > 0.001 {
		paso := 9 * dt
		if math.Abs(d) < paso {
			c.carrilX = obj
		} else {
			c.carrilX += math.Copysign(paso, d)
		}
	}
	// el salto
	if c.y > 0 || c.vy > 0 {
		c.vy -= gravedad * dt
		c.y += c.vy * dt
		if c.y <= 0 {
			c.y, c.vy = 0, 0
		}
	}
	c.agachado = math.Max(0, c.agachado-dt)
	c.iman = math.Max(0, c.iman-dt)
	c.escudo = math.Max(0, c.escudo-dt)
	c.temblor = math.Max(0, c.temblor-dt*3)
	for c.generado < c.recorrido+zLejos+6 {
		c.generar()
	}
	for c.palmeras < c.recorrido+zLejos+6 {
		c.palmeras += 11
		c.objs = append(c.objs, &objeto{tipo: oPalmera, carril: -1, s: c.palmeras}, &objeto{tipo: oPalmera, carril: 1, s: c.palmeras + 5.5})
	}
	c.chocar()
	// fuera lo que ya ha quedado atrás
	quedan := c.objs[:0]
	for _, o := range c.objs {
		if o.s > c.recorrido-4 && !(o.cogido && o.tipo >= oEstrella) {
			quedan = append(quedan, o)
		}
	}
	c.objs = quedan
}

// enCarril: si Zarpa está en ese carril (a mitad de cambio, cuenta el más cercano).
func (c *carrera) enCarril(l int) bool { return math.Abs(c.carrilX-float64(l)) < 0.45 }

func (c *carrera) chocar() {
	alto := altoZarpaU
	if c.agachado > 0 && c.y < 0.05 {
		alto = altoZarpaU * 0.48
	}
	for _, o := range c.objs {
		if o.cogido {
			continue
		}
		dz := o.s - c.recorrido
		switch o.tipo {
		case oEstrella, oIman, oEscudo:
			cerca := dz < 0.45 && dz > -0.45 && c.enCarril(o.carril) && math.Abs(c.y+alto/2-o.alto-0.15) < 0.75
			if o.tipo == oEstrella && c.iman > 0 && dz < 9 && dz > -0.5 {
				o.vuela = math.Min(1, o.vuela+0.12)
				cerca = cerca || o.vuela >= 1
			}
			if !cerca {
				continue
			}
			o.cogido = true
			switch o.tipo {
			case oEstrella:
				c.estrellas++
				c.avisos = append(c.avisos, "estrella")
			case oIman:
				c.iman = tIman
				c.avisos = append(c.avisos, "poder")
			case oEscudo:
				c.escudo = tEscudo
				c.avisos = append(c.avisos, "poder")
			}
		case oValla, oCartel, oCamion:
			largo := 0.3
			if o.tipo == oCamion {
				largo = largoCamion
			}
			if dz > 0.3 || dz < -largo || !c.enCarril(o.carril) {
				continue
			}
			golpe := false
			switch o.tipo {
			case oValla:
				golpe = c.y < altoValla-0.08
			case oCartel:
				golpe = c.y < altoCartel && c.y+alto > bajoCartel
			case oCamion:
				golpe = c.y < altoCamion
			}
			if !golpe {
				continue
			}
			if c.escudo > 0 {
				c.escudo, o.cogido = 0, true
				c.temblor = 0.6
				c.avisos = append(c.avisos, "escudo")
				continue
			}
			c.viva = false
			c.temblor = 1
			c.avisos = append(c.avisos, "choque")
			return
		}
	}
}

// ---- Lo que va saliendo ------------------------------------------------------------------------------

func (c *carrera) poner(t tipoObj, carril int, s, alto float64) {
	c.objs = append(c.objs, &objeto{tipo: t, carril: carril, s: s, alto: alto})
}

func (c *carrera) filaEstrellas(carril int, s float64, n int, arco bool) {
	for i := 0; i < n; i++ {
		alto := altoEstrella
		if arco { // por encima de una valla: en arco, como el salto
			t := float64(i) / float64(n-1)
			alto += 1.0 * math.Sin(t*math.Pi)
		}
		c.poner(oEstrella, carril, s+float64(i)*1.4, alto)
	}
}

func (c *carrera) generar() {
	r := c.rng
	sep := math.Max(9, c.vel*0.78)
	s := c.generado + sep
	switch x := r.Float64(); {
	case c.soloAdorno:
		c.generado = s + 30
	case c.recorrido < 20 && c.generado < 40: // el principio, tranquilo
		c.filaEstrellas(r.Intn(3)-1, s, 6, false)
		c.generado = s + 8
	case x < 0.2:
		c.filaEstrellas(r.Intn(3)-1, s, 5+r.Intn(4), false)
		c.generado = s + 9
	case x < 0.27 && c.generado-c.ultimoPoder > 160:
		t := oIman
		if r.Intn(2) == 0 {
			t = oEscudo
		}
		c.poner(t, r.Intn(3)-1, s, altoEstrella)
		c.ultimoPoder = s
		c.generado = s + 4
	default:
		// obstáculos: en cada carril, uno o nada; siempre queda por dónde pasar
		libre := r.Intn(3) - 1
		camiones := 0
		for l := -1; l <= 1; l++ {
			if l == libre && r.Float64() < 0.75 {
				if r.Float64() < 0.5 {
					c.filaEstrellas(l, s-2, 4, false)
				}
				continue
			}
			switch y := r.Float64(); {
			case y < 0.38:
				c.poner(oValla, l, s, 0)
				if r.Float64() < 0.35 {
					c.filaEstrellas(l, s-2.1, 4, true)
				}
			case y < 0.68:
				c.poner(oCartel, l, s, 0)
			case camiones < 2:
				c.poner(oCamion, l, s, 0)
				camiones++
			default:
				c.poner(oValla, l, s, 0)
			}
		}
		c.generado = s + largoCamion
	}
}

// cosas: lo que hay que pintar ahora.
func (c *carrera) cosas() []cosa {
	var out []cosa
	for _, o := range c.objs {
		if o.cogido && o.tipo != oPalmera {
			continue
		}
		z := o.s - c.recorrido
		if z > zLejos || z < -distCamara+0.6 {
			continue
		}
		k := cosa{z: z, x: float64(o.carril), alto: o.alto}
		switch o.tipo {
		case oValla:
			k.spr = "valla"
		case oCartel:
			k.spr = "cartel"
		case oCamion:
			k.spr = "camion"
		case oEstrella:
			k.spr = "estrella"
			if o.vuela > 0 { // hacia Zarpa
				k.z = z * (1 - o.vuela)
				k.x = k.x + (c.carrilX-k.x)*o.vuela
				k.alto = k.alto + (c.y+0.3-k.alto)*o.vuela
			}
			k.alto += 0.05 * math.Sin(c.tiempo*6+o.s)
		case oIman:
			k.spr = "iman"
			k.alto += 0.08 * math.Sin(c.tiempo*4)
		case oEscudo:
			k.spr = "escudo"
			k.alto += 0.08 * math.Sin(c.tiempo*4)
		case oPalmera:
			if z < 3 { // ya pasa por el borde de la pantalla: casi no se ve y es la más grande
				continue
			}
			k.spr, k.x = "palmeraI", -2.55
			if o.carril > 0 {
				k.spr, k.x = "palmeraD", 2.55
			}
		}
		out = append(out, k)
	}
	return out
}

func (c *carrera) postura() postura {
	return postura{fase: c.fase, salto: c.y > 0.05, agachado: c.agachado > 0 && c.y < 0.05, choque: !c.viva}
}

// escalarSobre: una imagen estirada a un rectángulo (la sombra de Zarpa, más pequeña al saltar).
func escalarSobre(dst *image.RGBA, r image.Rectangle, src *image.RGBA) {
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}
	xdraw.ApproxBiLinear.Scale(dst, r, src, src.Bounds(), draw.Over, nil)
}
