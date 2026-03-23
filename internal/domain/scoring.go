package domain

// bonusThresholds maps player count to minimum score for the +5 bonus
var bonusThresholds = map[int]int{
	3: 12,
	4: 15,
	5: 18,
	6: 20,
	7: 22,
	8: 24,
	9: 26,
}

// ClusterCount maps cluster_id -> list of (modelID, position) that have a word in that cluster
type clusterEntry struct {
	ModelID  ModelID
	Position int
}

// buildClusterMap creates cluster_id -> []clusterEntry from a completed game
func buildClusterMap(game *Game) map[int][]clusterEntry {
	cm := make(map[int][]clusterEntry)
	for modelID, player := range game.Players {
		if player.Error != "" {
			continue
		}
		for _, w := range player.Words {
			if w.ClusterID == 0 {
				// Unmatched word — treat each as its own virtual negative cluster using position hack
				// We skip unmatched (cluster 0 means no match)
				continue
			}
			cm[w.ClusterID] = append(cm[w.ClusterID], clusterEntry{ModelID: modelID, Position: w.Position})
		}
	}
	return cm
}

// ApplyClusters assigns cluster IDs to all words in the game based on the judge's output
func ApplyClusters(game *Game, clusters map[string]int) {
	game.Clusters = clusters
	for _, player := range game.Players {
		for i, w := range player.Words {
			if cid, ok := clusters[w.Text]; ok {
				player.Words[i].ClusterID = cid
			} else {
				player.Words[i].ClusterID = 0 // no match
			}
		}
	}
}

// ApplyStrictClusters assigns cluster IDs based on exact string matching only (pre-judge).
// Each unique word gets its own cluster; exact matches share a cluster.
func ApplyStrictClusters(game *Game) {
	wordToCluster := make(map[string]int)
	nextCluster := 1

	// First pass: assign clusters based on exact lowercase match
	for _, player := range game.Players {
		for _, w := range player.Words {
			key := normalizeWord(w.Text)
			if _, exists := wordToCluster[key]; !exists {
				wordToCluster[key] = nextCluster
				nextCluster++
			}
		}
	}

	// Second pass: apply clusters
	for _, player := range game.Players {
		for i, w := range player.Words {
			key := normalizeWord(w.Text)
			player.Words[i].ClusterID = wordToCluster[key]
		}
	}
}

func normalizeWord(w string) string {
	result := []rune{}
	for _, r := range w {
		if r >= 'A' && r <= 'Z' {
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

// CalculateScores computes both Unanimo and Synchronicity scores for all players
func CalculateScores(game *Game) {
	clusterMap := buildClusterMap(game)

	// Build: clusterID -> count of distinct models in that cluster
	clusterModelCount := make(map[int]int)
	for cid, entries := range clusterMap {
		seen := make(map[ModelID]bool)
		for _, e := range entries {
			seen[e.ModelID] = true
		}
		clusterModelCount[cid] = len(seen)
	}

	// Build: (clusterID, position) -> count of distinct models
	type clusterPos struct {
		ClusterID int
		Position  int
	}
	positionMap := make(map[clusterPos]int)
	for cid, entries := range clusterMap {
		posModels := make(map[int]map[ModelID]bool)
		for _, e := range entries {
			if posModels[e.Position] == nil {
				posModels[e.Position] = make(map[ModelID]bool)
			}
			posModels[e.Position][e.ModelID] = true
		}
		for pos, models := range posModels {
			positionMap[clusterPos{cid, pos}] = len(models)
		}
	}

	numPlayers := game.ActivePlayerCount()

	for modelID, player := range game.Players {
		if player.Error != "" {
			continue
		}

		unanimoTotal := 0
		syncTotal := 0

		for _, w := range player.Words {
			if w.ClusterID == 0 {
				// No match — 0 points
				continue
			}

			modelCount := clusterModelCount[w.ClusterID]
			if modelCount < 2 {
				// Unique word — no points
				continue
			}

			// Unanimo base: points = number of models sharing this cluster
			basePoints := modelCount
			unanimoTotal += basePoints

			// Synchronicity: multiply by number of models at same position
			key := clusterPos{w.ClusterID, w.Position}
			posCount := positionMap[key]
			if posCount >= 2 {
				syncTotal += basePoints * posCount
			} else {
				syncTotal += basePoints
			}
		}

		// Apply +5 bonus if threshold met
		bonus := 0
		if threshold, ok := bonusThresholds[numPlayers]; ok && unanimoTotal >= threshold {
			bonus = 5
		}

		player.UnanimoScore = unanimoTotal
		player.Bonus = bonus
		player.TotalUnanimoScore = unanimoTotal + bonus
		player.SynchronicityScore = syncTotal
		game.Players[modelID] = player
	}
}

// GetStrictMatches returns words that are exact matches across multiple models
// Returns map: normalized_word -> []ModelID
func GetStrictMatches(game *Game) map[string][]ModelID {
	wordModels := make(map[string][]ModelID)
	for modelID, player := range game.Players {
		for _, w := range player.Words {
			key := normalizeWord(w.Text)
			wordModels[key] = append(wordModels[key], modelID)
		}
	}

	matches := make(map[string][]ModelID)
	for word, models := range wordModels {
		if len(models) >= 2 {
			matches[word] = models
		}
	}
	return matches
}
