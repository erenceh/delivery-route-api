package domain

// Immutable geographic coordinates (longitude, latitude).
type Coordinates struct {
	Lon float64
	Lat float64
}

// Return coordinates as [lon, lat] for external API compatibility.
func (c Coordinates) CoordsToList() []float64 { return []float64{c.Lon, c.Lat} }
