package domain

import "math/rand"

// ConceptBank is a large set of interesting concepts for the game
var ConceptBank = []string{
	// Nature
	"Ocean", "Forest", "Mountain", "Desert", "River",
	"Storm", "Rainbow", "Volcano", "Cave", "Island",
	// Animals
	"Tiger", "Eagle", "Dolphin", "Wolf", "Butterfly",
	"Snake", "Elephant", "Penguin", "Octopus", "Owl",
	// Foods & Drinks
	"Pizza", "Coffee", "Chocolate", "Sushi", "Honey",
	"Wine", "Bread", "Spice", "Fruit", "Salt",
	// Emotions & Abstract
	"Joy", "Fear", "Dream", "Memory", "Freedom",
	"Love", "Time", "Silence", "Power", "Justice",
	// Technology
	"Robot", "Internet", "Space", "Algorithm", "Virus",
	"Rocket", "Microscope", "Battery", "Signal", "Code",
	// Art & Culture
	"Music", "Theater", "Dance", "Painting", "Poetry",
	"Cinema", "Fashion", "Myth", "Symbol", "Ritual",
	// Places & Things
	"Library", "Bridge", "Market", "Tower", "Garden",
	"Mirror", "Clock", "Compass", "Key", "Map",
	// Science
	"Atom", "Light", "Gravity", "Evolution", "Crystal",
	"Fire", "Ice", "Magnet", "Echo", "Shadow",
	// Sports & Games
	"Chess", "Marathon", "Surfing", "Archery", "Circus",
	"Climbing", "Sailing", "Wrestling", "Polo", "Diving",
	// Society
	"School", "Hospital", "Prison", "Festival", "War",
	"Trade", "Vote", "Tax", "Border", "Tradition",
}

// GetRandomConcepts returns n unique random concepts from the bank
func GetRandomConcepts(n int) []string {
	if n >= len(ConceptBank) {
		result := make([]string, len(ConceptBank))
		copy(result, ConceptBank)
		return result
	}

	indices := rand.Perm(len(ConceptBank))
	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = ConceptBank[indices[i]]
	}
	return result
}
