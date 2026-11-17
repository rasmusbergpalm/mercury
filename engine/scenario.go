package engine

import (
	"fmt"
	"math"
	"strings"
)

// Recipe is a firm template: output, base TFP and Cobb-Douglas factors.
type Recipe struct {
	Firm    string
	Output  string
	TFP     float64
	Factors []Factor
}

// RawRecipes are land-bound producers; terrain decides where they exist.
var RawRecipes = []Recipe{
	{"Farm", "grain", 2.0, []Factor{Labor(0.6)}},
	{"Potato farm", "potatoes", 2.5, []Factor{Labor(0.6)}},
	{"Ranch", "meat", 1.2, []Factor{Labor(0.6)}},
	{"Fishery", "fish", 1.5, []Factor{Labor(0.6)}},
	{"Forestry", "wood", 2.0, []Factor{Labor(0.6)}},
	{"Coal mine", "coal", 2.0, []Factor{Labor(0.6)}},
	{"Iron mine", "iron ore", 1.5, []Factor{Labor(0.6)}},
	{"Sheep farm", "wool", 1.5, []Factor{Labor(0.6)}},
	{"Cotton plantation", "cotton", 1.5, []Factor{Labor(0.6)}},
	{"Tea plantation", "tea", 1.0, []Factor{Labor(0.6)}},
	{"Sugar plantation", "sugar cane", 2.0, []Factor{Labor(0.6)}},
	{"Vineyard", "grapes", 1.5, []Factor{Labor(0.6)}},
	{"Silk farm", "silk", 0.8, []Factor{Labor(0.6)}},
	{"Clay pit", "clay", 2.0, []Factor{Labor(0.6)}},
}

// ProcessingRecipes can be set up anywhere. "fuel" = wood or coal, "fiber" = wool or cotton.
var ProcessingRecipes = []Recipe{
	{"Bakery", "bread", 2.0, []Factor{Labor(0.3), Input("grain", 0.4), Comp("fuel", 0.1)}},
	{"Brewery", "beer", 1.5, []Factor{Labor(0.3), Input("grain", 0.4)}},
	{"Mill", "cloth", 1.5, []Factor{Labor(0.3), Comp("fiber", 0.4)}},
	{"Tailor", "clothes", 1.5, []Factor{Labor(0.3), Input("cloth", 0.4)}},
	{"Smelter", "iron", 1.0, []Factor{Labor(0.2), Input("iron ore", 0.3), Comp("fuel", 0.2)}},
	{"Smithy", "ironware", 1.2, []Factor{Labor(0.3), Input("iron", 0.3), Comp("fuel", 0.1)}},
	{"Carpenter", "furniture", 1.2, []Factor{Labor(0.3), Input("wood", 0.4)}},
	{"Refinery", "sugar", 1.2, []Factor{Labor(0.2), Input("sugar cane", 0.4), Comp("fuel", 0.1)}},
	{"Winery", "wine", 1.2, []Factor{Labor(0.3), Input("grapes", 0.4)}},
	{"Couturier", "fine clothes", 1.0, []Factor{Labor(0.4), Input("silk", 0.3)}},
	{"Kiln", "porcelain", 1.0, []Factor{Labor(0.4), Input("clay", 0.2), Comp("fuel", 0.2)}},
	{"Transport", "transport", 2.0, []Factor{Labor(0.6)}},
}

// GameRecipes exist only in game worlds (with nations): services for
// building, research and moving goods, war industry, and industrial firms
// unlocked by technology.
var GameRecipes = []Recipe{
	{"Construction", "construction", 1.5, []Factor{Labor(0.5), Input("wood", 0.2)}},
	{"Armory", "arms", 1.0, []Factor{Labor(0.3), Input("iron", 0.25), Input("wood", 0.1)}},
	{"Foundry", "artillery", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1)}},
	{"Munitions plant", "munitions", 1.5, []Factor{Labor(0.3), Input("iron", 0.2), Input("coal", 0.2)}},
	{"Motor works", "vehicles", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1)}},
	{"Aircraft factory", "aircraft", 0.8, []Factor{Labor(0.4), Input("wood", 0.15), Input("cloth", 0.1), Input("iron", 0.1)}},
	{"Toolmaker", "tools", 1.2, []Factor{Labor(0.3), Input("iron", 0.25), Input("wood", 0.1)}},
	{"Engine works", "steam engines", 0.8, []Factor{Labor(0.3), Input("iron", 0.3), Input("coal", 0.1), Input("tools", 0.1)}},
	{"Chemical works", "explosives", 1.0, []Factor{Labor(0.3), Input("coal", 0.2), Input("clay", 0.1)}},
	{"Fertilizer works", "fertilizer", 1.2, []Factor{Labor(0.3), Input("coal", 0.15), Input("clay", 0.15)}},
	{"Power station", "electricity", 2.0, []Factor{Labor(0.2), Input("coal", 0.3), Input("steam engines", 0.1)}},
}

// serviceFirms are the firms in the service sector (the rest of the
// non-primary firms are industry).
var serviceFirms = map[string]bool{"Transport": true, "Construction": true, "University": true}

// Methods are other ways of making what a firm makes, which a tech makes
// available (Tech.EnableFirms): each is a firm of its own beside the old
// one, with its own recipe and levels. More productive, they need
// machinery and fuel the old one doesn't, so they spread as owners find
// them worth building (and the old one comes back where the machinery is
// dear). A land-bound one shares its land with the old one: it grows by
// taking the old one's levels over. Like the machines of the period, a
// method replaces labour with machinery and fuel, and keeps the old one's
// share for its capital (one less its exponents): with less, it would
// leave less profit a level and never spread; with more, the economy
// would need far more capital than 19th-century economies had.
var MethodRecipes = []struct {
	Base string // the firm whose method it is
	Recipe
	// Cap is the levels a land-bound method can work, if more than the
	// old one's (RawMaxLevels): steam pumps and blasting reach seams the
	// old mines couldn't.
	Cap int
}{
	// Every firm that exists in 1820 has a method for each later era: the
	// steam age (1840s-60s), steel, electricity and chemistry (1870s-90s)
	// and the motor age (1900s-10s). Each keeps the capital share of the
	// one before (one less its exponents) and turns labour into that
	// era's machinery and fuel: engines and coal, then iron, power,
	// fertilizer and explosives, then vehicles and oil. A firm's wage bill
	// is its labour exponent times its sales, so each frees workers even
	// where demand for its good doesn't grow. Each is about a tenth more
	// profitable a level than the one before at the prices of its day
	// (its productivity is set so), so it spreads as owners refit, as the
	// machines of the period did.
	{"Farm", Recipe{"Seed-drill farm", "grain", 3.32, []Factor{Labor(0.44), Input("tools", 0.16)}}, 0},                                                                                            // era 2: Seed drill
	{"Farm", Recipe{"Fertilized farm", "grain", 4.61, []Factor{Labor(0.32), Input("tools", 0.1), Input("fertilizer", 0.18)}}, 0},                                                                  // era 3: Fertilizers
	{"Farm", Recipe{"Tractor farm", "grain", 5.59, []Factor{Labor(0.18), Input("vehicles", 0.2), Input("oil", 0.1), Input("fertilizer", 0.12)}}, 0},                                               // era 4: Mechanized agriculture
	{"Potato farm", Recipe{"Horse-hoe potato farm", "potatoes", 3.9, []Factor{Labor(0.48), Input("tools", 0.12)}}, 0},                                                                             // era 2: Seed drill
	{"Potato farm", Recipe{"Fertilized potato farm", "potatoes", 5.46, []Factor{Labor(0.36), Input("fertilizer", 0.16), Input("tools", 0.08)}}, 0},                                                // era 3: Fertilizers
	{"Potato farm", Recipe{"Motorized potato farm", "potatoes", 6.64, []Factor{Labor(0.24), Input("fertilizer", 0.14), Input("vehicles", 0.12), Input("oil", 0.1)}}, 0},                           // era 4: Mechanized agriculture
	{"Ranch", Recipe{"Fodder ranch", "meat", 2.19, []Factor{Labor(0.38), Input("grain", 0.14), Input("tools", 0.08)}}, 0},                                                                         // era 2: Seed drill
	{"Ranch", Recipe{"Fenced ranch", "meat", 2.94, []Factor{Labor(0.3), Input("grain", 0.14), Input("iron", 0.08), Input("tools", 0.08)}}, 0},                                                     // era 3: Steel
	{"Ranch", Recipe{"Motorized ranch", "meat", 3.13, []Factor{Labor(0.22), Input("grain", 0.14), Input("vehicles", 0.14), Input("oil", 0.1)}}, 0},                                                // era 4: Mechanized agriculture
	{"Fishery", Recipe{"Ice-packed fishery", "fish", 2.25, []Factor{Labor(0.5), Input("tools", 0.1)}}, 0},                                                                                         // era 2: Railways
	{"Fishery", Recipe{"Steam trawler", "fish", 4.4, []Factor{Labor(0.3), Input("steam engines", 0.18), Input("coal", 0.12)}}, 0},                                                                 // era 3: Steel
	{"Fishery", Recipe{"Motor trawler", "fish", 3.14, []Factor{Labor(0.22), Input("vehicles", 0.2), Input("oil", 0.18)}}, 0},                                                                      // era 4: Internal combustion
	{"Forestry", Recipe{"Steam sawmill", "wood", 5.44, []Factor{Labor(0.32), Input("steam engines", 0.16), Input("coal", 0.12)}}, 0},                                                              // era 2: Steam power
	{"Forestry", Recipe{"Band-saw mill", "wood", 6.9, []Factor{Labor(0.24), Input("steam engines", 0.14), Input("coal", 0.1), Input("tools", 0.12)}}, 0},                                          // era 3: Steel
	{"Forestry", Recipe{"Motor logging", "wood", 5.57, []Factor{Labor(0.18), Input("vehicles", 0.18), Input("oil", 0.12), Input("tools", 0.12)}}, 0},                                              // era 4: Internal combustion
	{"Coal mine", Recipe{"Steam coal mine", "coal", 4.04, []Factor{Labor(0.45), Input("steam engines", 0.1), Input("coal", 0.05)}}, 16},                                                           // era 2: Steam power
	{"Coal mine", Recipe{"Mechanized coal mine", "coal", 5.63, []Factor{Labor(0.32), Input("steam engines", 0.08), Input("electricity", 0.12), Input("tools", 0.08)}}, 16},                        // era 3: Electricity
	{"Coal mine", Recipe{"Conveyor coal mine", "coal", 6.35, []Factor{Labor(0.22), Input("electricity", 0.18), Input("tools", 0.1), Input("steam engines", 0.1)}}, 16},                            // era 4: Mass production
	{"Iron mine", Recipe{"Steam iron mine", "iron ore", 3.03, []Factor{Labor(0.45), Input("steam engines", 0.1), Input("coal", 0.05)}}, 6},                                                        // era 2: Steam power
	{"Iron mine", Recipe{"Blasting iron mine", "iron ore", 3.38, []Factor{Labor(0.4), Input("explosives", 0.12), Input("steam engines", 0.08)}}, 8},                                               // era 3: Industrial chemistry
	{"Iron mine", Recipe{"Motor ore mine", "iron ore", 3.8, []Factor{Labor(0.26), Input("explosives", 0.12), Input("vehicles", 0.12), Input("oil", 0.1)}}, 8},                                     // era 4: Internal combustion
	{"Sheep farm", Recipe{"Bred-flock sheep farm", "wool", 2.11, []Factor{Labor(0.48), Input("grain", 0.12)}}, 0},                                                                                 // era 2: Seed drill
	{"Sheep farm", Recipe{"Machine-shorn sheep farm", "wool", 2.91, []Factor{Labor(0.38), Input("grain", 0.12), Input("tools", 0.1)}}, 0},                                                         // era 3: Steel
	{"Sheep farm", Recipe{"Motorized sheep station", "wool", 3.61, []Factor{Labor(0.28), Input("grain", 0.12), Input("vehicles", 0.12), Input("oil", 0.08)}}, 0},                                  // era 4: Mechanized agriculture
	{"Cotton plantation", Recipe{"Steam-gin plantation", "cotton", 2.92, []Factor{Labor(0.46), Input("steam engines", 0.08), Input("coal", 0.06)}}, 0},                                            // era 2: Steam power
	{"Cotton plantation", Recipe{"Fertilized cotton plantation", "cotton", 4.03, []Factor{Labor(0.36), Input("steam engines", 0.08), Input("coal", 0.06), Input("fertilizer", 0.1)}}, 0},          // era 3: Fertilizers
	{"Cotton plantation", Recipe{"Mechanized cotton plantation", "cotton", 3.77, []Factor{Labor(0.24), Input("fertilizer", 0.1), Input("vehicles", 0.14), Input("oil", 0.12)}}, 0},                // era 4: Mechanized agriculture
	{"Tea plantation", Recipe{"Estate tea garden", "tea", 1.56, []Factor{Labor(0.48), Input("tools", 0.12)}}, 0},                                                                                  // era 2: Seed drill
	{"Tea plantation", Recipe{"Tea estate with rollers", "tea", 2.82, []Factor{Labor(0.36), Input("steam engines", 0.12), Input("coal", 0.06), Input("tools", 0.06)}}, 0},                         // era 3: Steel
	{"Tea plantation", Recipe{"Mechanized tea estate", "tea", 2.6, []Factor{Labor(0.26), Input("vehicles", 0.14), Input("oil", 0.1), Input("tools", 0.1)}}, 0},                                    // era 4: Mechanized agriculture
	{"Sugar plantation", Recipe{"Steam-mill plantation", "sugar cane", 5.09, []Factor{Labor(0.34), Input("steam engines", 0.12), Input("coal", 0.14)}}, 0},                                        // era 2: Steam power
	{"Sugar plantation", Recipe{"Fertilized cane plantation", "sugar cane", 6.35, []Factor{Labor(0.28), Input("steam engines", 0.1), Input("coal", 0.1), Input("fertilizer", 0.12)}}, 0},          // era 3: Fertilizers
	{"Sugar plantation", Recipe{"Mechanized cane plantation", "sugar cane", 5.38, []Factor{Labor(0.2), Input("fertilizer", 0.12), Input("vehicles", 0.16), Input("oil", 0.12)}}, 0},               // era 4: Mechanized agriculture
	{"Vineyard", Recipe{"Trellised vineyard", "grapes", 2.25, []Factor{Labor(0.5), Input("tools", 0.1)}}, 0},                                                                                      // era 2: Seed drill
	{"Vineyard", Recipe{"Grafted vineyard", "grapes", 3.05, []Factor{Labor(0.42), Input("tools", 0.1), Input("fertilizer", 0.08)}}, 0},                                                            // era 3: Industrial chemistry
	{"Vineyard", Recipe{"Motorized vineyard", "grapes", 4.25, []Factor{Labor(0.3), Input("tools", 0.1), Input("fertilizer", 0.08), Input("vehicles", 0.08), Input("oil", 0.04)}}, 0},              // era 4: Mechanized agriculture
	{"Silk farm", Recipe{"Steam filature", "silk", 1.56, []Factor{Labor(0.46), Input("steam engines", 0.08), Input("coal", 0.06)}}, 0},                                                            // era 2: Steam filatures
	{"Silk farm", Recipe{"Selected-egg silk farm", "silk", 2.03, []Factor{Labor(0.4), Input("steam engines", 0.08), Input("coal", 0.06), Input("fertilizer", 0.06)}}, 0},                          // era 3: Industrial chemistry
	{"Silk farm", Recipe{"Electric filature", "silk", 2.02, []Factor{Labor(0.3), Input("electricity", 0.14), Input("tools", 0.1), Input("fertilizer", 0.06)}}, 0},                                 // era 4: Mass production
	{"Clay pit", Recipe{"Steam clay works", "clay", 3.89, []Factor{Labor(0.46), Input("steam engines", 0.08), Input("coal", 0.06)}}, 0},                                                           // era 2: Steam power
	{"Clay pit", Recipe{"Blasted clay pit", "clay", 4.8, []Factor{Labor(0.38), Input("explosives", 0.1), Input("steam engines", 0.12)}}, 0},                                                       // era 3: Industrial chemistry
	{"Clay pit", Recipe{"Excavator clay pit", "clay", 5.1, []Factor{Labor(0.26), Input("vehicles", 0.16), Input("oil", 0.1), Input("explosives", 0.08)}}, 0},                                      // era 4: Internal combustion
	{"Oil well", Recipe{"Rotary-drilled well", "oil", 2.67, []Factor{Labor(0.4), Input("steam engines", 0.1), Input("tools", 0.1)}}, 0},                                                           // era 4: Internal combustion
	{"Bakery", Recipe{"Steam bakery", "bread", 2.87, []Factor{Labor(0.22), Input("grain", 0.4), Input("coal", 0.12), Input("steam engines", 0.06)}}, 0},                                           // era 2: Steam power
	{"Bakery", Recipe{"Electric bakery", "bread", 3.34, []Factor{Labor(0.16), Input("grain", 0.4), Input("electricity", 0.1), Input("tools", 0.04), Input("coal", 0.1)}}, 0},                      // era 3: Electricity
	{"Bakery", Recipe{"Industrial bakery", "bread", 3.48, []Factor{Labor(0.1), Input("grain", 0.4), Input("electricity", 0.16), Input("tools", 0.06), Input("coal", 0.08)}}, 0},                   // era 4: Mass production
	{"Brewery", Recipe{"Steam brewery", "beer", 2.44, []Factor{Labor(0.18), Input("grain", 0.4), Input("coal", 0.07), Input("steam engines", 0.05)}}, 0},                                          // era 2: Steam power
	{"Brewery", Recipe{"Refrigerated brewery", "beer", 2.92, []Factor{Labor(0.14), Input("grain", 0.4), Input("coal", 0.06), Input("steam engines", 0.04), Input("electricity", 0.06)}}, 0},       // era 3: Refrigeration
	{"Brewery", Recipe{"Bottling brewery", "beer", 2.96, []Factor{Labor(0.1), Input("grain", 0.4), Input("electricity", 0.1), Input("tools", 0.06), Input("coal", 0.04)}}, 0},                     // era 4: Mass production
	{"Mill", Recipe{"Textile factory", "cloth", 2.53, []Factor{Labor(0.15), Comp("fiber", 0.4), Input("coal", 0.1), Input("steam engines", 0.05)}}, 0},                                            // era 2: Mechanized textiles
	{"Mill", Recipe{"Ring-spinning mill", "cloth", 2.6, []Factor{Labor(0.12), Comp("fiber", 0.4), Input("electricity", 0.1), Input("tools", 0.08)}}, 0},                                           // era 3: Electricity
	{"Mill", Recipe{"Automatic-loom mill", "cloth", 2.75, []Factor{Labor(0.08), Comp("fiber", 0.4), Input("electricity", 0.14), Input("tools", 0.08)}}, 0},                                        // era 4: Assembly line
	{"Tailor", Recipe{"Sewing-machine tailor", "clothes", 1.96, []Factor{Labor(0.24), Input("cloth", 0.4), Input("tools", 0.06)}}, 0},                                                             // era 2: Interchangeable parts
	{"Tailor", Recipe{"Garment factory", "clothes", 2.47, []Factor{Labor(0.18), Input("cloth", 0.4), Input("electricity", 0.08), Input("tools", 0.04)}}, 0},                                       // era 3: Electricity
	{"Tailor", Recipe{"Garment assembly line", "clothes", 2.72, []Factor{Labor(0.12), Input("cloth", 0.4), Input("electricity", 0.1), Input("tools", 0.08)}}, 0},                                  // era 4: Assembly line
	{"Smelter", Recipe{"Hot-blast furnace", "iron", 1.21, []Factor{Labor(0.18), Input("iron ore", 0.3), Input("coal", 0.2), Input("steam engines", 0.02)}}, 0},                                    // era 2: Steam power
	{"Smelter", Recipe{"Steel works", "iron", 1.43, []Factor{Labor(0.17), Input("iron ore", 0.3), Input("coal", 0.18), Input("steam engines", 0.05)}}, 0},                                         // era 3: Steel
	{"Smelter", Recipe{"Electric arc furnace", "iron", 1.49, []Factor{Labor(0.1), Input("iron ore", 0.3), Input("electricity", 0.15), Input("coal", 0.1), Input("tools", 0.05)}}, 0},              // era 4: Mass production
	{"Smithy", Recipe{"Steam-hammer forge", "ironware", 1.72, []Factor{Labor(0.22), Input("iron", 0.3), Input("coal", 0.12), Input("steam engines", 0.06)}}, 0},                                   // era 2: Steam power
	{"Smithy", Recipe{"Electric smithy", "ironware", 1.55, []Factor{Labor(0.2), Input("iron", 0.3), Input("electricity", 0.2)}}, 0},                                                               // era 3: Electricity
	{"Smithy", Recipe{"Drop-forge works", "ironware", 1.83, []Factor{Labor(0.12), Input("iron", 0.3), Input("electricity", 0.2), Input("tools", 0.08)}}, 0},                                       // era 4: Mass production
	{"Carpenter", Recipe{"Steam joinery", "furniture", 1.8, []Factor{Labor(0.22), Input("wood", 0.4), Input("coal", 0.04), Input("steam engines", 0.04)}}, 0},                                     // era 2: Steam power
	{"Carpenter", Recipe{"Electric joinery", "furniture", 1.84, []Factor{Labor(0.18), Input("wood", 0.4), Input("electricity", 0.12)}}, 0},                                                        // era 3: Electricity
	{"Carpenter", Recipe{"Furniture factory", "furniture", 2.12, []Factor{Labor(0.1), Input("wood", 0.4), Input("electricity", 0.14), Input("tools", 0.06)}}, 0},                                  // era 4: Mass production
	{"Refinery", Recipe{"Vacuum-pan refinery", "sugar", 1.65, []Factor{Labor(0.12), Input("sugar cane", 0.4), Input("coal", 0.12), Input("steam engines", 0.06)}}, 0},                             // era 2: Steam power
	{"Refinery", Recipe{"Centrifugal refinery", "sugar", 1.92, []Factor{Labor(0.08), Input("sugar cane", 0.4), Input("coal", 0.08), Input("electricity", 0.1), Input("steam engines", 0.04)}}, 0}, // era 3: Electricity
	{"Refinery", Recipe{"Continuous refinery", "sugar", 1.91, []Factor{Labor(0.06), Input("sugar cane", 0.4), Input("electricity", 0.14), Input("coal", 0.06), Input("tools", 0.04)}}, 0},         // era 4: Mass production
	{"Winery", Recipe{"Steam-press winery", "wine", 1.7, []Factor{Labor(0.24), Input("grapes", 0.4), Input("coal", 0.03), Input("steam engines", 0.03)}}, 0},                                      // era 2: Steam power
	{"Winery", Recipe{"Pasteurizing winery", "wine", 2.13, []Factor{Labor(0.18), Input("grapes", 0.4), Input("coal", 0.06), Input("steam engines", 0.06)}}, 0},                                    // era 3: Research laboratories
	{"Winery", Recipe{"Industrial winery", "wine", 2.05, []Factor{Labor(0.12), Input("grapes", 0.4), Input("electricity", 0.1), Input("tools", 0.08)}}, 0},                                        // era 4: Mass production
	{"Couturier", Recipe{"Sewing-machine atelier", "fine clothes", 1.39, []Factor{Labor(0.32), Input("silk", 0.3), Input("tools", 0.08)}}, 0},                                                     // era 2: Interchangeable parts
	{"Couturier", Recipe{"Electric atelier", "fine clothes", 1.79, []Factor{Labor(0.26), Input("silk", 0.3), Input("electricity", 0.08), Input("tools", 0.06)}}, 0},                               // era 3: Electricity
	{"Couturier", Recipe{"Couture workshop line", "fine clothes", 1.99, []Factor{Labor(0.2), Input("silk", 0.3), Input("electricity", 0.12), Input("tools", 0.08)}}, 0},                           // era 4: Assembly line
	{"Kiln", Recipe{"Factory kiln", "porcelain", 1.68, []Factor{Labor(0.25), Input("clay", 0.2), Input("coal", 0.25), Input("steam engines", 0.05), Input("tools", 0.05)}}, 0},                    // era 2: Interchangeable parts
	{"Kiln", Recipe{"Tunnel kiln", "porcelain", 1.92, []Factor{Labor(0.18), Input("clay", 0.2), Input("coal", 0.3), Input("steam engines", 0.06), Input("tools", 0.06)}}, 0},                      // era 3: Industrial chemistry
	{"Kiln", Recipe{"Electric kiln", "porcelain", 1.66, []Factor{Labor(0.12), Input("clay", 0.2), Input("electricity", 0.3), Input("tools", 0.08), Input("steam engines", 0.1)}}, 0},              // era 4: Mass production
	{"Transport", Recipe{"Steam transport", "transport", 4.04, []Factor{Labor(0.45), Input("steam engines", 0.1), Input("coal", 0.05)}}, 0},                                                       // era 2: Locomotives
	{"Transport", Recipe{"Heavy rail freight", "transport", 5.9, []Factor{Labor(0.34), Input("steam engines", 0.14), Input("coal", 0.08), Input("iron", 0.04)}}, 0},                               // era 3: Rail network
	{"Transport", Recipe{"Motor transport", "transport", 4.47, []Factor{Labor(0.26), Input("vehicles", 0.2), Input("oil", 0.14)}}, 0},                                                             // era 4: Motor trucks
	{"Construction", Recipe{"Steam shovels", "construction", 2.37, []Factor{Labor(0.42), Input("wood", 0.2), Input("steam engines", 0.05), Input("coal", 0.03)}}, 0},                              // era 2: Locomotives
	{"Construction", Recipe{"Steam cranes", "construction", 3.57, []Factor{Labor(0.32), Input("wood", 0.12), Input("iron", 0.12), Input("steam engines", 0.08), Input("coal", 0.06)}}, 0},         // era 3: Engineering
	{"Construction", Recipe{"Motorized construction", "construction", 3.28, []Factor{Labor(0.22), Input("wood", 0.1), Input("iron", 0.14), Input("vehicles", 0.14), Input("oil", 0.1)}}, 0},       // era 4: Internal combustion
	{"Armory", Recipe{"Armory system", "arms", 1.35, []Factor{Labor(0.22), Input("iron", 0.25), Input("wood", 0.1), Input("tools", 0.08)}}, 0},                                                    // era 2: Interchangeable parts
	{"Armory", Recipe{"Steel arms works", "arms", 1.64, []Factor{Labor(0.16), Input("iron", 0.28), Input("wood", 0.05), Input("tools", 0.08), Input("coal", 0.08)}}, 0},                           // era 3: Steel
	{"Armory", Recipe{"Mass-production armory", "arms", 1.59, []Factor{Labor(0.1), Input("iron", 0.28), Input("wood", 0.04), Input("tools", 0.1), Input("electricity", 0.13)}}, 0},                // era 4: Mass production
	{"Foundry", Recipe{"Steam foundry", "artillery", 1.15, []Factor{Labor(0.22), Input("iron", 0.3), Input("coal", 0.12), Input("steam engines", 0.06)}}, 0},                                      // era 2: Steam power
	{"Foundry", Recipe{"Steel gun works", "artillery", 1.32, []Factor{Labor(0.16), Input("iron", 0.32), Input("coal", 0.14), Input("steam engines", 0.08)}}, 0},                                   // era 3: Steel
	{"Foundry", Recipe{"Electric foundry", "artillery", 1.1, []Factor{Labor(0.1), Input("iron", 0.32), Input("electricity", 0.18), Input("tools", 0.1)}}, 0},                                      // era 4: Mass production
	{"Toolmaker", Recipe{"Machine-tool works", "tools", 1.71, []Factor{Labor(0.25), Input("iron", 0.25), Input("steam engines", 0.08), Input("coal", 0.07)}}, 0},                                  // era 2: Interchangeable parts
	{"Toolmaker", Recipe{"Electric tool works", "tools", 2, []Factor{Labor(0.16), Input("iron", 0.25), Input("electricity", 0.14), Input("steam engines", 0.04), Input("tools", 0.06)}}, 0},       // era 3: Electricity
	{"Toolmaker", Recipe{"Tool assembly line", "tools", 1.84, []Factor{Labor(0.1), Input("iron", 0.25), Input("electricity", 0.2), Input("tools", 0.1)}}, 0},                                      // era 4: Assembly line
	{"Engine works", Recipe{"Compound-engine works", "steam engines", 1.04, []Factor{Labor(0.2), Input("iron", 0.3), Input("coal", 0.1), Input("tools", 0.14), Input("electricity", 0.06)}}, 0},   // era 3: Steel
	{"Engine works", Recipe{"Engine assembly line", "steam engines", 1.05, []Factor{Labor(0.12), Input("iron", 0.3), Input("tools", 0.16), Input("electricity", 0.16), Input("coal", 0.06)}}, 0},  // era 4: Assembly line
	{"Chemical works", Recipe{"Electrochemical works", "explosives", 1.17, []Factor{Labor(0.18), Input("coal", 0.14), Input("clay", 0.1), Input("electricity", 0.18)}}, 0},                        // era 4: Mass production
	{"Fertilizer works", Recipe{"Haber-process plant", "fertilizer", 1.38, []Factor{Labor(0.16), Input("coal", 0.2), Input("clay", 0.06), Input("electricity", 0.18)}}, 0},                        // era 4: Mass production
	{"Power station", Recipe{"Turbine power station", "electricity", 2.37, []Factor{Labor(0.1), Input("coal", 0.32), Input("steam engines", 0.14), Input("tools", 0.04)}}, 0},                     // era 4: Mass production
	{"Munitions plant", Recipe{"Shell production line", "munitions", 1.76, []Factor{Labor(0.16), Input("iron", 0.22), Input("coal", 0.16), Input("electricity", 0.16)}}, 0},                       // era 4: Mass production
	{"Motor works", Recipe{"Assembly-line works", "vehicles", 0.9, []Factor{Labor(0.18), Input("iron", 0.3), Input("coal", 0.05), Input("electricity", 0.17)}}, 0},                                // era 4: Assembly line
}

// GameRawRecipes are land-bound producers that exist only in game worlds,
// with where they can be set up.
var GameRawRecipes = []Recipe{
	{"Oil well", "oil", 1.5, []Factor{Labor(0.6)}},
}

// GameTerrainRaw adds game-only raw producers to terrains.
var GameTerrainRaw = map[string][]Bonus{
	"tropical": {{"Oil well", 1.5}},
	"plains":   {{"Oil well", 0.7}},
	"desert":   {{"Oil well", 1.5}},
	"steppe":   {{"Oil well", 0.7}},
	"tundra":   {{"Oil well", 0.5}},
}

// FirmTech maps a firm to the tech that enables it (from Tech.EnableFirms).
// In game worlds such firms only work for nations that know the tech.
var FirmTech = func() map[string]string {
	m := map[string]string{}
	for _, t := range Techs {
		for _, f := range t.EnableFirms {
			m[f] = t.Name
		}
	}
	return m
}()

// RawMaxLevels caps building levels of land-bound producers (land is scarce).
const RawMaxLevels = 4

// Bonus is a raw producer available on a terrain, with a TFP multiplier.
type Bonus struct {
	Firm string
	Mult float64
}

// Terrain is a tile type.
type Terrain struct {
	Name     string
	Symbol   byte
	MoveCost float64 // cost of moving through the tile
	Raw      []Bonus
}

// Terrains are the tile types. Iron ore is mined best in mountains; elsewhere
// poorer deposits (hill ores, bog iron, laterite) can be worked at a fraction
// of the productivity, so a nation cut off from mountains can still arm, dearly.
var Terrains = []Terrain{
	{"plains", 'P', 1.0, []Bonus{{"Farm", 1.5}, {"Potato farm", 1.2}, {"Ranch", 1.3},
		{"Sheep farm", 1.0}, {"Clay pit", 1.0}, {"Iron mine", 0.25}}},
	{"forest", 'F', 1.0, []Bonus{{"Forestry", 1.8}, {"Potato farm", 0.8}, {"Ranch", 0.7}, {"Iron mine", 0.3}}},
	{"hills", 'H', 1.0, []Bonus{{"Sheep farm", 1.5}, {"Vineyard", 1.5}, {"Coal mine", 1.0},
		{"Clay pit", 1.3}, {"Potato farm", 1.0}, {"Iron mine", 0.5}}},
	{"mountains", 'M', 1.0, []Bonus{{"Coal mine", 1.5}, {"Iron mine", 1.5}, {"Sheep farm", 0.8}}},
	{"tropical", 'T', 1.0, []Bonus{{"Cotton plantation", 1.5}, {"Sugar plantation", 1.5},
		{"Tea plantation", 1.5}, {"Silk farm", 1.2}, {"Farm", 0.6}, {"Iron mine", 0.25}}},
	// the dry grasslands (the Great Plains, the Pontic and Kazakh steppe, the
	// Sahel, Patagonia): herds above all, and grain where the rain allows
	{"steppe", 'S', 1.0, []Bonus{{"Ranch", 1.6}, {"Sheep farm", 1.4}, {"Farm", 1.0},
		{"Potato farm", 0.5}, {"Clay pit", 0.8}, {"Iron mine", 0.25}}},
	// desert: nomads' herds, oases' grain, clay; little else (until oil)
	{"desert", 'D', 1.0, []Bonus{{"Ranch", 0.3}, {"Sheep farm", 0.4}, {"Farm", 0.2},
		{"Clay pit", 0.6}, {"Iron mine", 0.25}}},
	// tundra beyond the tree line: reindeer herds, scrub timber, and the ores
	// and coal of the northern shields
	{"tundra", 'U', 1.0, []Bonus{{"Ranch", 0.2}, {"Forestry", 0.3}, {"Iron mine", 0.4},
		{"Coal mine", 0.6}}},
}

// The ocean surrounds the map. A tile on the map's edge is coastal whatever
// its terrain: it can fish, ships carry goods past it cheaply (CoastMove,
// less with naval techs) and it can build a port.
const CoastMove = 0.25

// CoastRaw are the producers every coastal tile has on top of its terrain's.
var CoastRaw = []Bonus{{"Fishery", 1.5}}

// TerrainBySymbol looks a terrain up by its map symbol.
func TerrainBySymbol(sym byte) (Terrain, error) {
	for _, t := range Terrains {
		if t.Symbol == sym {
			return t, nil
		}
	}
	return Terrain{}, fmt.Errorf("unknown terrain symbol %q", sym)
}

// DefaultMap is a 6x6 hex map, "odd-r" offset layout (odd rows are shifted
// half a hex right).
var DefaultMap = []string{
	"MMHFFP",
	"HHFPPP",
	"FFPPHP",
	"PPPTTH",
	"PPTTTP",
	"HPPTPF",
}

// HexNeighbors lists the neighbours of (r, c) in an odd-r offset grid.
func HexNeighbors(r, c, rows, cols int) [][2]int {
	var deltas [][2]int
	if r%2 == 0 {
		deltas = [][2]int{{-1, -1}, {-1, 0}, {0, -1}, {0, 1}, {1, -1}, {1, 0}}
	} else {
		deltas = [][2]int{{-1, 0}, {-1, 1}, {0, -1}, {0, 1}, {1, 0}, {1, 1}}
	}
	var out [][2]int
	for _, d := range deltas {
		rr, cc := r+d[0], c+d[1]
		if rr >= 0 && rr < rows && cc >= 0 && cc < cols {
			out = append(out, [2]int{rr, cc})
		}
	}
	return out
}

func makeFirms(t Terrain, coastal, game bool) []*Firm {
	var firms []*Firm
	addRaw := func(recipes []Recipe, bonuses []Bonus, site string) {
		for _, rc := range recipes {
			for _, b := range bonuses {
				if b.Firm == rc.Firm {
					f := NewFirm(rc.Firm, G(rc.Output), rc.TFP*b.Mult, rc.Factors, 50)
					f.MaxLevels, f.Sector = RawMaxLevels, Primary
					f.Site, f.SiteMult = site, b.Mult
					firms = append(firms, f)
				}
			}
		}
	}
	addRaw(RawRecipes, t.Raw, t.Name)
	if coastal {
		addRaw(RawRecipes, CoastRaw, "the coast")
	}
	if game {
		addRaw(GameRawRecipes, GameTerrainRaw[t.Name], t.Name)
	}
	recipes := ProcessingRecipes
	if game {
		recipes = append(append([]Recipe(nil), recipes...), GameRecipes...)
	}
	for _, rc := range recipes {
		f := NewFirm(rc.Firm, G(rc.Output), rc.TFP, rc.Factors, 50)
		if serviceFirms[rc.Firm] {
			f.Sector = Service
		}
		firms = append(firms, f)
	}
	if game {
		for k := Public(0); k < NumServices; k++ {
			firms = append(firms, newServiceFirm(k))
		}
		for _, f := range firms {
			f.Requires = FirmTech[f.Name]
			f.Enabled = f.Requires == ""
			// the methods techs bring, at the site's productivity; a land-bound
			// one may work as much of the land as its Cap
			for _, mr := range MethodRecipes {
				if mr.Base != f.Name {
					continue
				}
				site := 1.0
				if f.SiteMult > 0 {
					site = f.SiteMult
				}
				m := newMethod(mr.Firm, mr.TFP*site, mr.Factors)
				m.Requires, m.Enabled = FirmTech[mr.Firm], false
				if f.Sector == Primary {
					m.Cap = max(RawMaxLevels, mr.Cap)
				}
				f.Methods = append(f.Methods, m)
			}
		}
	}
	return firms
}

// BuildWorld builds the world for a hex map without nations (the model the
// Python reference specifies). transportCost scales every hop; +Inf means
// autarky (no trade).
func BuildWorld(transportCost float64, gameMap []string) (*World, error) {
	return buildWorld(transportCost, gameMap, nil, false)
}

// Sea marks a map cell with no tile: open water. Tiles next to it (or to
// the map's edge) are coast.
const Sea = '~'

// The cost of a hop between neighbouring tiles is the average of the two
// tiles' movement costs (land 1, coastal 1/4, less with infrastructure). Traders
// only run between neighbours; longer trade happens by re-export along a chain
// of tiles.
// names, if given, names the tiles in order (row by row, skipping the sea);
// otherwise a tile is named by its row and column.
func buildWorld(transportCost float64, gameMap []string, names []string, game bool) (*World, error) {
	if len(gameMap) == 0 {
		return nil, fmt.Errorf("empty map")
	}
	rows, cols := len(gameMap), len(gameMap[0])
	for r, row := range gameMap {
		if len(row) != cols {
			return nil, fmt.Errorf("map row %d has %d columns, want %d", r, len(row), cols)
		}
	}
	grid := make([][]*Region, rows)
	var regions []*Region
	for r := 0; r < rows; r++ {
		if len(gameMap[r]) != cols {
			return nil, fmt.Errorf("map row %d has %d columns, want %d", r, len(gameMap[r]), cols)
		}
		grid[r] = make([]*Region, cols)
		for c := 0; c < cols; c++ {
			if gameMap[r][c] == Sea {
				continue
			}
			t, err := TerrainBySymbol(gameMap[r][c])
			if err != nil {
				return nil, err
			}
			land := 0
			for _, nb := range HexNeighbors(r, c, rows, cols) {
				if gameMap[nb[0]][nb[1]] != Sea {
					land++
				}
			}
			coastal := land < 6
			name := fmt.Sprintf("%d%d", r, c)
			if names != nil {
				if len(regions) >= len(names) {
					return nil, fmt.Errorf("the map has more tiles than names")
				}
				name = names[len(regions)]
			}
			// people start with 10 a head of spending a day, in savings of a
			// twentieth (spendShare)
			reg := NewRegion(len(regions), name, makeFirms(t, coastal, game), 2, 50, 500/spendShare)
			if game {
				addOwnerClasses(reg)
				reg.Educated = startEducated
			}
			reg.Row, reg.Col, reg.Terrain, reg.BaseMove, reg.Coastal = r, c, t.Name, t.MoveCost, coastal
			grid[r][c] = reg
			regions = append(regions, reg)
		}
	}

	if len(regions) == 0 {
		return nil, fmt.Errorf("the map has no land")
	}
	if names != nil && len(names) != len(regions) {
		return nil, fmt.Errorf("the map has %d tiles but %d names", len(regions), len(names))
	}
	var traders []*Trader
	var links []Link
	for _, a := range regions {
		for _, nb := range HexNeighbors(a.Row, a.Col, rows, cols) {
			b := grid[nb[0]][nb[1]]
			if b == nil {
				continue // sea
			}
			hop := (a.MoveCost() + b.MoveCost()) / 2
			if a.ID < b.ID {
				links = append(links, Link{a.ID, b.ID, hop})
			}
			if !math.IsInf(transportCost, 1) {
				tradable := Tradable
				if game {
					tradable = GameTradable
				}
				for _, g := range tradable {
					traders = append(traders, NewTrader(g, a, b, hop, transportCost))
				}
			}
		}
	}
	return NewWorld(rows, cols, regions, traders, links), nil
}

// NationSpec describes a nation in a game scenario.
type NationSpec struct {
	Symbol byte // marks its tiles in the nation map
	Name   string
	Color  string
}

// GameSpec is a game scenario: a terrain map plus who owns which tile.
type GameSpec struct {
	Map           []string
	NationMap     []string // same shape as Map; '.' = unowned
	Nations       []NationSpec
	TransportCost float64
	TaxRate       float64
	AI            bool  // nations are run by the computer until a player takes one
	Seed          int64 // draws the computer nations' temperaments and their whims
	// Names names the tiles, row by row (skipping the sea); empty: by
	// their row and column.
	Names []string `json:",omitempty"`
}

// DefaultGame is the default game scenario on DefaultMap: three nations of
// four or five tiles, about equal in people and output once the map has
// settled (Montara's mountains are poor, so it has a rich forest too); the
// rest of the map is unowned and can be annexed.
var DefaultGame = GameSpec{
	Map: DefaultMap,
	NationMap: []string{
		"AA..BB",
		"AA.BB.",
		"A.....",
		"......",
		"...CC.",
		"...CC.",
	},
	Nations: []NationSpec{
		{'A', "Montara", "#b8322e"},
		{'B', "Verdania", "#2c5aa0"},
		{'C', "Thalassa", "#c08a1e"},
	},
	TransportCost: 1,
	TaxRate:       0.1,
	Names:         NameTiles(DefaultMap, 1820),
}

// BuildGame builds a game world: regions owned by nations that tax their pops,
// trade under tariffs, research technology and build infrastructure, with
// Construction and University firms on every tile, investing firms and
// migrating people.
//
// The game opens on a settled world: see SettleYears.
func BuildGame(spec GameSpec) (*World, error) {
	return buildGame(spec, true, nil)
}

// BuildGameProgress is BuildGame reporting how far the settling has got
// (days done of the total), which takes a while on a big map.
func BuildGameProgress(spec GameSpec, progress func(done, total int)) (*World, error) {
	return buildGame(spec, true, progress)
}

// baseGameWorld builds a game's map with every tile urban, before any
// nation or settling.
func baseGameWorld(spec GameSpec) (*World, error) {
	if math.IsInf(spec.TransportCost, 0) || math.IsNaN(spec.TransportCost) || spec.TransportCost < 0 {
		return nil, fmt.Errorf("transport cost must be a non-negative number")
	}
	w, err := buildWorld(spec.TransportCost, spec.Map, spec.Names, true)
	if err != nil {
		return nil, err
	}
	for _, r := range w.Regions {
		r.Urban = true
		r.updateAgglomeration()
	}
	return w, nil
}

// buildGame builds a game, settled first or not (a save restores its own
// state over it).
func buildGame(spec GameSpec, settle bool, progress func(done, total int)) (*World, error) {
	w, err := baseGameWorld(spec)
	if err != nil {
		return nil, err
	}
	if len(spec.NationMap) != w.Rows {
		return nil, fmt.Errorf("nation map has %d rows, want %d", len(spec.NationMap), w.Rows)
	}
	tiles := len(w.Regions)
	report := func(done int) {
		if progress != nil {
			progress(done, PrepareDays(tiles))
		}
	}
	if settle {
		b, err := settledBase(spec, func(done, _ int) { report(done) })
		if err != nil {
			return nil, err
		}
		w.settleFrom(b)
	}
	w.Investment, w.Migration, w.Demography = true, true, true
	w.Spec = &spec
	bySymbol := map[byte]*Nation{}
	for i, ns := range spec.Nations {
		n := newNation(i, ns.Name, ns.Color, len(spec.Nations), spec.TaxRate)
		n.AI = spec.AI
		n.Temper = drawTemperament(spec.Seed, i)
		n.seed = spec.Seed
		if err := n.SetTaxRate(spec.TaxRate); err != nil {
			return nil, err
		}
		bySymbol[ns.Symbol] = n
		w.Nations = append(w.Nations, n)
	}
	for _, r := range w.Regions {
		row := spec.NationMap[r.Row]
		if len(row) != w.Cols {
			return nil, fmt.Errorf("nation map row %d has %d columns, want %d", r.Row, len(row), w.Cols)
		}
		if row[r.Col] == '.' {
			continue
		}
		n, ok := bySymbol[row[r.Col]]
		if !ok {
			return nil, fmt.Errorf("unknown nation symbol %q", row[r.Col])
		}
		r.Nation = n
		n.Regions = append(n.Regions, r)
	}
	for _, n := range w.Nations {
		n.applyTech()
	}
	w.initLoyalties()
	w.refreshAllHops()
	if settle {
		w.burnIn(BurnInDays(tiles), func(done int) { report(SettleDays(tiles) + done) })
	}
	return w, nil
}

// Owner classes in game worlds: each is this share of a tile's people and
// starts with this share of its cash (so owners are much richer per head).
const (
	ownerPeopleShare = 0.04
	ownerCashShare   = 0.10
)

// addOwnerClasses turns part of a tile's people into an owner pop.
func addOwnerClasses(r *Region) {
	pop, cash := r.Population(), 0.0
	for _, p := range r.Pops {
		cash += p.Cash
	}
	for _, p := range r.Pops {
		p.Size *= 1 - ownerPeopleShare
		p.Cash *= 1 - ownerCashShare
	}
	p := NewPop(ownerPeopleShare*pop, ownerCashShare*cash)
	p.Class = Owners
	r.Pops = append(r.Pops, p)
	r.popDemand = append(r.popDemand, make([]float64, NumGoods))
}

// A tech's description lists the production methods it brings, from the
// method table, so that the two can't disagree.
func init() {
	method := map[string]bool{}
	for _, m := range MethodRecipes {
		method[m.Firm] = true
	}
	for i := range Techs {
		var names []string
		for _, f := range Techs[i].EnableFirms {
			if method[f] {
				names = append(names, strings.ToLower(f[:1])+f[1:])
			}
		}
		if len(names) == 0 {
			continue
		}
		list := "new methods: " + strings.Join(names, ", ")
		if Techs[i].Effect == "" {
			Techs[i].Effect = strings.ToUpper(list[:1]) + list[1:]
		} else {
			Techs[i].Effect += "; " + list
		}
	}
}
