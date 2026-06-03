package main

import (
	"fmt"
	"math/rand"
	"time"
)

// names.go assigns each connecting player an anonymous, throwaway identity.
// There are no accounts: a fresh "AdjectiveAnimalNN" handle is minted on every
// WebSocket connect (see hub.handleConnect) and forgotten on disconnect. Adapted
// from virusgame's names.go, dropping the bot-name generator.

var adjectives = []string{
	"Brave", "Clever", "Wild", "Swift", "Bold", "Mighty", "Mystic", "Noble",
	"Fierce", "Gentle", "Silent", "Rapid", "Calm", "Proud", "Wise", "Happy",
	"Lucky", "Sneaky", "Cunning", "Bright", "Dark", "Golden", "Silver", "Royal",
	"Ancient", "Modern", "Quick", "Slow", "Tiny", "Giant", "Cool", "Hot",
}

var animals = []string{
	"Octopus", "Tiger", "Phoenix", "Dragon", "Eagle", "Wolf", "Bear", "Fox",
	"Lion", "Hawk", "Shark", "Panther", "Raven", "Falcon", "Cobra", "Viper",
	"Lynx", "Owl", "Dolphin", "Whale", "Rhino", "Jaguar", "Cheetah", "Leopard",
	"Puma", "Otter", "Badger", "Raccoon", "Moose", "Buffalo", "Bison", "Elk",
}

var rng *rand.Rand

func init() {
	rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// GenerateRandomName creates a random username of the form Adjective+Animal+NN
// (a fixed two-digit suffix), e.g. "SwiftFalcon42".
func GenerateRandomName() string {
	adjective := adjectives[rng.Intn(len(adjectives))]
	animal := animals[rng.Intn(len(animals))]
	number := rng.Intn(90) + 10 // 10-99, always two digits
	return fmt.Sprintf("%s%s%d", adjective, animal, number)
}
