package game

import "github.com/rasmusbergpalm/mercury/economy"

// The game is played on the economy of package economy; these are the
// names from it the game uses most, under their own names.

type (
	GoodID    = economy.GoodID
	Good      = economy.Good
	Firm      = economy.Firm
	Method    = economy.Method
	Factor    = economy.Factor
	Expansion = economy.Expansion
	Sector    = economy.Sector
	Pop       = economy.Pop
	Class     = economy.Class
	Job       = economy.Job
	Trader    = economy.Trader
	Market    = economy.Market
	GDPChain  = economy.GDPChain
	Recipe    = economy.Recipe
	Terrain   = economy.Terrain
	Bonus     = economy.Bonus
	Need      = economy.Need
	Tier      = economy.Tier
	Composite = economy.Composite
	Link      = economy.Link
)

const (
	TicksPerYear  = economy.TicksPerYear
	Sea           = economy.Sea
	NumClasses    = economy.NumClasses
	Workers       = economy.Workers
	Owners        = economy.Owners
	Primary       = economy.Primary
	Industry      = economy.Industry
	Service       = economy.Service
	Basic         = economy.Basic
	NumTiers      = economy.NumTiers
	Luxury        = economy.Luxury
	StartYear     = economy.StartYear
	LevelCost     = economy.LevelCost
	CityMult      = economy.CityMult
	MinPopulation = economy.MinPopulation
	minPopulation = economy.MinPopulation
)

var (
	Goods           = economy.Goods
	TierOf          = economy.TierOf
	Date            = economy.Date
	NumGoods        = economy.NumGoods
	Transport       = economy.Transport
	Construction    = economy.Construction
	Research        = economy.Research
	Electricity     = economy.Electricity
	ConsumerGoods   = economy.ConsumerGoods
	Tradable        = economy.Tradable
	MethodRecipes   = economy.MethodRecipes
	FirmTech        = economy.FirmTech
	DefaultMap      = economy.DefaultMap
	Terrains        = economy.Terrains
	RawMaxLevels    = economy.RawMaxLevels
	G               = economy.G
	NameTiles       = economy.NameTiles
	Labor           = economy.Labor
	Input           = economy.Input
	NewFirm         = economy.NewFirm
	NewTrader       = economy.NewTrader
	NewPop          = economy.NewPop
	HexNeighbors    = economy.HexNeighbors
	TerrainBySymbol = economy.TerrainBySymbol
	IsService       = economy.IsService
	sizeOf          = economy.SizeOf
	movable         = economy.Movable
)
