// Package game is an example of building on Mercury's economy (package
// economy): a strategy game, not yet named, in which nations own the map's
// tiles, tax their people, set tariffs, research technology, build roads,
// railways and ports, run public services (administration, schools, public
// health, universities), raise armies and go to war, form alliances, and
// are played by people or by the computer.
//
// A game World wraps an economy.World, and each game Region an
// economy.Region. The game plays its day by running the economy's steps
// and acting between them: each morning it sets the economy's levers from
// the nations (their technology, tariffs, embargoes, taxes and policies),
// buys in the markets for them, takes their taxes and duties from the
// tiles' accounts, and settles their treasuries, services and wars.
package game
