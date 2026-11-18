// Package game is Mercury, a strategy game played on the economy of
// package economy: nations own the map's tiles, tax their people, set
// tariffs, research technology, build roads, railways and ports, run public
// services (administration, schools, public health, universities), raise
// armies and go to war, form alliances, and are played by people or by the
// computer.
//
// A game World wraps an economy.World, and each game Region an
// economy.Region: a tile's nation is its Government in the economy, and the
// World's hooks run the nations' day (their purchases, their treasury,
// their armies) at fixed points of the economy's.
package game
