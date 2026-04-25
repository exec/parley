package dm

import (
	"math/rand/v2"
)

// Themed default-name buckets for newly-created group DMs. Picked at
// create time based on member count; not regenerated when the group
// crosses size thresholds. Owner can override the chosen name freely.

var namesSize3 = []string{
	"Trifecta", "The Three Musketeers", "The Three Stooges", "Triumvirate",
	"Triple Threat", "The Holy Trinity", "Three Amigos", "Power Trio",
	"Three's Company", "The Triplets", "Trinity", "Three Wise Monkeys",
	"Three Wise Men", "The Threesome", "Triad",
}

var namesSizeSmall = []string{ // 4-10
	"The Squad", "The Crew", "The Gang", "The Posse", "The Pack",
	"The Bunch", "The Lot", "The Cabal", "The Coalition", "The Clique",
	"The Inner Circle", "The League", "The Round Table", "The Avengers",
	"The Fellowship", "The Bunch of Misfits", "The Usual Suspects",
	"The A-Team", "The Wolfpack", "The Council",
}

var namesSizeLarge = []string{ // 11+
	"Small Army", "The Horde", "The Mob", "The Multitude", "The Battalion",
	"Gangsters! From the Far Side of the Moon!",
	"The Rebellion", "The Conspiracy",
	"The Convention", "Tiny Township", "The Flotilla",
	"Society of Definitely Not Up To Anything",
	"The Migration", "The Caravan", "The Symposium", "Town Hall Without The Hall",
	"The Stadium Wave", "The Cargo Cult",
}

// PickGroupName returns a random name from the bucket appropriate for the size.
func PickGroupName(memberCount int) string {
	bucket := bucketFor(memberCount)
	if len(bucket) == 0 {
		return "Group Chat"
	}
	return bucket[rand.IntN(len(bucket))]
}

func bucketFor(memberCount int) []string {
	switch {
	case memberCount <= 3:
		return namesSize3
	case memberCount <= 10:
		return namesSizeSmall
	default:
		return namesSizeLarge
	}
}
