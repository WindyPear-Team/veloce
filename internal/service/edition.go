package service

var editionProvider func() string

func RegisterEditionProvider(provider func() string) {
	editionProvider = provider
}

func CurrentEdition() string {
	if editionProvider == nil {
		return "community"
	}
	switch editionProvider() {
	case "premium":
		return "premium"
	default:
		return "community"
	}
}
