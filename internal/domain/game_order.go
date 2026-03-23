package domain

import "sort"

// LegacyPlayerOrder returns a stable column order for games without PlayerOrder (older saves).
func LegacyPlayerOrder(g *Game) []ModelID {
	if len(g.PlayerOrder) > 0 {
		return g.PlayerOrder
	}
	var out []ModelID
	for _, id := range ModelOrder {
		if _, ok := g.Players[id]; ok {
			out = append(out, id)
		}
	}
	if len(out) > 0 {
		return out
	}
	for id := range g.Players {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}
