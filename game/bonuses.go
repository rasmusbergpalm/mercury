package game

import (
	"fmt"
	"strings"
)

// FirmBonuses explains the firm's productivity factor by factor: its recipe's
// base, its site (terrain or coast), each technology of its nation, the
// tile's size (industry and services), its schooling, and the crowding
// malus: how much less productive it is than it would be with one more
// building level.
func FirmBonuses(r *Region, f *Firm) []Modifier {
	n := r.Nation
	site := 1.0
	if f.SiteMult > 0 {
		site = f.SiteMult
	}
	mi := f.Main()
	out := []Modifier{{"Base", f.Methods[mi].BaseTFP / site, "the recipe's productivity", false}}
	if f.SiteMult > 0 {
		out = append(out, Modifier{cap1(f.Site), f.SiteMult, "how well the site suits it", false})
	}
	if n != nil {
		for i, t := range Techs {
			if !n.Known[i] {
				continue
			}
			full := techFirmMult(&t, f, f.Methods[mi])
			if full == 1 {
				continue
			}
			note := "technology"
			if a := n.Adoption(i); a < 1 {
				note = fmt.Sprintf("technology, %.0f%% adopted (of ×%.2f)", 100*a, full)
			}
			out = append(out, Modifier{t.Name, 1 + (full-1)*n.Adoption(i), note, false})
		}
	}
	if f.Agglom != 1 {
		out = append(out, Modifier{"Tile size", f.Agglom, "industry and services gain from the people around them", false})
	}
	if m := r.WorkMult(); m < 1 {
		out = append(out, Modifier{"Allegiance", m, "the tile's people don't yet stand by their new rulers", false})
	}
	if m := r.eduMult(f); m != 1 {
		out = append(out, Modifier{"Schooling", m, "the tile's educated share", false})
	}
	if m := f.LandMult(); m != 1 {
		out = append(out, Modifier{"Land", m, fmt.Sprintf("its land, %d levels' worth, shared over %d", f.LandLevels(), f.Levels), false})
	}
	if f.Levels > 0 {
		m := f.CrowdingMult()
		note := fmt.Sprintf("%d levels; one more would make every worker and input %.0f%% more productive", f.Levels, 100*(m-1))
		if f.AtCap() {
			note = fmt.Sprintf("%d levels, the most it can have here", f.Levels)
		}
		out = append(out, Modifier{"Crowding", 1 / m, note, false})
	}
	return out
}

func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
