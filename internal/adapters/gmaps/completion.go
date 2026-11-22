package gmaps

type CompletionTracker interface {
	SeedDiscovered(inputID string, placesFound int) error
}
