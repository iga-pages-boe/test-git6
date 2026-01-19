package config

const DefaultServicePSM = "data.ti.bigbrother"

var (
	psm     = DefaultServicePSM
	logDir  string
	port    int
	cluster string
)

func LogDir() string {
	return logDir
}
func SetLogDir(nLogDir string) {
	logDir = nLogDir
}

func PSM() string {
	return psm
}

func SetPSM(nPsm string) {
	psm = nPsm
}
