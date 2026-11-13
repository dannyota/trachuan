package source

type Pin struct {
	Key    string `yaml:"key" json:"key"`
	Number string `yaml:"number" json:"number"`
	Expand string `yaml:"expand" json:"expand"`
	Fetch  string `yaml:"fetch" json:"fetch"`
	Rel    string `yaml:"rel" json:"rel"`
	Since  string `yaml:"since" json:"since"`
}

func (l List) Pin(key string) (Pin, bool) {
	for _, pin := range l.VBPL.Pinned {
		if pin.Key == key {
			return pin, true
		}
	}
	return Pin{}, false
}
