package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Un /sys de mentira con la ranura (ff370000.mmc, la tarjeta de Panther OS) y la memoria de dentro
// (ff390000.mmc, eMMC), como en la XF35H.
type sysFalso struct {
	t    *testing.T
	raiz string
}

func nuevoSys(t *testing.T) *sysFalso {
	r := t.TempDir()
	viejo := raizSys
	raizSys = r
	t.Cleanup(func() { raizSys = viejo })
	if err := os.Symlink(r, filepath.Join(r, "prueba-enlace")); err != nil {
		t.Skip("aquí no se pueden hacer enlaces simbólicos:", err)
	}
	return &sysFalso{t, r}
}

func (s *sysFalso) escribir(ruta, texto string) {
	p := filepath.Join(s.raiz, ruta)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(texto+"\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sysFalso) enlace(destino, ruta string) {
	p := filepath.Join(s.raiz, ruta)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.Remove(p)
	if err := os.Symlink(filepath.Join(s.raiz, destino), p); err != nil {
		s.t.Fatal(err)
	}
}

// meter: una tarjeta en un lector, con sus particiones (maj:min 179:n).
func (s *sysFalso) meter(host, mmc, disco, cid, tipo string, primero int, particiones int) {
	tarjeta := "devices/platform/" + host + "/mmc_host/" + mmc + "/" + mmc + ":0001"
	bloque := tarjeta + "/block/" + disco
	s.escribir(tarjeta+"/cid", cid)
	s.escribir(tarjeta+"/type", tipo)
	s.escribir(bloque+"/size", "15523840")
	s.escribir(bloque+"/dev", "179:"+itoa(primero))
	s.enlace(tarjeta, bloque+"/device")
	for i := 1; i <= particiones; i++ {
		p := bloque + "/" + disco + "p" + itoa(i)
		s.escribir(p+"/dev", "179:"+itoa(primero+i))
		s.enlace(p, "dev/block/179:"+itoa(primero+i))
	}
	s.enlace(bloque, "block/"+disco)
	s.escribir("devices/platform/"+host+"/.lector", "")
}

func (s *sysFalso) sacar(disco string) {
	_ = os.Remove(filepath.Join(s.raiz, "block", disco))
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestTarjetas(t *testing.T) {
	s := nuevoSys(t)
	s.meter("ff370000.mmc", "mmc0", "mmcblk0", "panther-cid", "SD", 0, 2)
	s.meter("ff390000.mmc", "mmc1", "mmcblk1", "emmc-cid", "MMC", 32, 1)
	s.meter("ff390000.mmc", "mmc1", "mmcblk1boot0", "emmc-cid", "MMC", 64, 0)

	// «/» está en mmcblk0p2 (179:2).
	panther, err := tarjetaDeDispositivo(179, 2)
	if err != nil {
		t.Fatal(err)
	}
	if panther.disco != "mmcblk0" || panther.cid != "panther-cid" || panther.tipo != "SD" || !strings.HasSuffix(panther.host, "ff370000.mmc") {
		t.Fatalf("tarjeta de Panther OS mal: %+v", panther)
	}
	if panther.tam != 15523840*512 {
		t.Fatalf("tamaño mal: %d", panther.tam)
	}

	// En su lector solo está ella (la eMMC y sus «boot» van aparte).
	hay := tarjetasEn(panther.host)
	if len(hay) != 1 || hay[0].disco != "mmcblk0" {
		t.Fatalf("en la ranura: %+v", hay)
	}
	if otra, esta := clasificar(hay, panther.cid); otra != nil || !esta {
		t.Fatal("con la de Panther OS dentro no hay otra")
	}
	if d := describirRanura(hay, panther.cid); d != "la tarjeta de Panther OS" {
		t.Fatal(d)
	}

	// Se saca: la ranura queda vacía.
	s.sacar("mmcblk0")
	hay = tarjetasEn(panther.host)
	if len(hay) != 0 || describirRanura(hay, panther.cid) != "vacía" {
		t.Fatalf("tendría que estar vacía: %+v", hay)
	}

	// Se mete la de la flashcard: sale como mmcblk2, con otro CID y una partición.
	s.meter("ff370000.mmc", "mmc0", "mmcblk2", "flashcard-cid", "SD", 96, 1)
	hay = tarjetasEn(panther.host)
	otra, esta := clasificar(hay, panther.cid)
	if otra == nil || esta || otra.disco != "mmcblk2" {
		t.Fatalf("la de la flashcard no se ve: %+v", hay)
	}
	if d := describirRanura(hay, panther.cid); d != "otra tarjeta (8 GB)" {
		t.Fatal(d)
	}
	nombre, d, err := particion("mmcblk2")
	if err != nil || nombre != "mmcblk2p1" || d != "179:97" {
		t.Fatalf("partición: %q %q %v", nombre, d, err)
	}

	// Una tarjeta sin particiones: la tarjeta entera.
	s.meter("ff370000.mmc", "mmc0", "mmcblk3", "otra-cid", "SD", 128, 0)
	if nombre, d, err := particion("mmcblk3"); err != nil || nombre != "mmcblk3" || d != "179:128" {
		t.Fatalf("sin particiones: %q %q %v", nombre, d, err)
	}

	// Y vuelve la de Panther OS.
	s.sacar("mmcblk2")
	s.sacar("mmcblk3")
	s.meter("ff370000.mmc", "mmc0", "mmcblk4", "panther-cid", "SD", 160, 2)
	if otra, esta := clasificar(tarjetasEn(panther.host), panther.cid); otra != nil || !esta {
		t.Fatal("no se ve que ha vuelto la de Panther OS")
	}
}

func TestSoloLectura(t *testing.T) {
	m := `/dev/root / ext4 rw,noatime 0 0
devtmpfs /dev devtmpfs rw,relatime 0 0
tmpfs /dev/shm tmpfs rw,nosuid,nodev 0 0
/dev/mmcblk0p1 /boot vfat rw,relatime 0 0
/dev/mmcblk0p3 /roms exfat rw,relatime 0 0
/dev/mmcblk1p1 /mnt/emmc ext4 rw 0 0
`
	if todoSoloLectura(m, "mmcblk0") {
		t.Fatal("aún está en lectura y escritura")
	}
	got := montajesDe(m, "mmcblk0")
	if len(got) != 3 || got["/"] || got["/boot"] || got["/roms"] {
		t.Fatalf("montajes: %v", got)
	}
	ro := strings.NewReplacer(" rw,noatime", " ro,noatime", "vfat rw", "vfat ro", "exfat rw", "exfat ro").Replace(m)
	if !todoSoloLectura(ro, "mmcblk0") {
		t.Fatalf("tendría que estar todo en solo lectura:\n%s", ro)
	}
	if todoSoloLectura("", "mmcblk0") {
		t.Fatal("sin montajes no se puede saber")
	}
}
