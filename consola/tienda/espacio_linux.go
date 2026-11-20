package main

import "syscall"

// espacioLibre: lo que queda en el disco de esa carpeta (-1 si no se sabe).
func espacioLibre(dir string) int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
