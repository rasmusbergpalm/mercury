// Package economy is a deterministic, spatial, agent-based economy: a hex
// map of tiles, each with its own people, firms and markets, linked by
// merchants who pay to carry goods from tile to tile.
//
// Each day, on every tile:
//
//   - firms plan with Cobb-Douglas recipes, hiring labour and buying inputs
//     up to where they pay (a firm's wage bill is its labour exponent times
//     its sales), and owners add building levels where a level would repay
//     its cost, and refit levels to better production methods;
//   - people spend their income on needs in tiers (basic, intermediate,
//     luxury), with goods within a need substituting at a constant
//     elasticity, as much as their budget and today's prices make worth it;
//   - one market clears for each good, and the labour market for work, and
//     prices move with the excess of demand over supply;
//   - each tile's trading house runs a route for every good to every
//     neighbour, buying where it is cheap and selling where it is dear while
//     the margin covers the transport (a service like any other, bought in
//     the market) its bulk and the distance take;
//   - firms produce, pay out what they don't need to their owners, and wear
//     out; people move towards the tiles where they live better, and grow
//     in number when they eat well.
//
// The same world stepped the same number of days gives bit-identical
// state: random choices are hashes of the seed and stable keys, and the
// tiles' own work, which runs in parallel, settles what it owes others in a
// fixed order afterwards.
//
// What the economy makes and where is not the engine's: the goods, needs,
// recipes and terrains are data, in the JSON files of data/, and a map is
// a list of rows of terrain symbols. The data that comes with the package
// is an example economy, Europe around 1820, and DefaultMap a small example
// map. BuildWorld builds a world from them: every good and firm, with
// production methods that wait for the technology they need, and workers
// and owners in every tile.
//
// Anything outside the economy (a model of policy, a strategy game) acts
// on it by setting its levers (see levers.go: taxes, tariffs and embargoes,
// outside buyers, the cost of capital, productivity) and by reading the
// accounts where what it collects piles up. A caller that acts within a
// day runs its steps (BeginDay, PlanTrade, Markets, Ship, Produce, EndDay)
// itself and acts between them. The economy never calls out.
package economy
