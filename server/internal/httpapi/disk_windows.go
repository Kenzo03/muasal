package httpapi

// Windows runs only as a development host, so disk figures stay empty there.

func sameDisk(a, b string) bool { return false }

func diskUse(volume, path string) DiskUse {
	return DiskUse{Volume: DiskUseVolume(volume), Path: path}
}
