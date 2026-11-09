package config

import "waystone-web/models"

const karrowWorldID = "5b7e9d21-8c3a-4f6e-a1d4-7c2e9b0f3a61"

// InitialWorld is the demo Codex world; the Age of Sojourn game is set in it.
var InitialWorld = models.World{
	ID:   karrowWorldID,
	Name: "The Karrow Marches",
	Description: "A cold border country of toll roads, dark waystones and villages that have stopped answering.\n\n" +
		"Start with *The Karrow Road*, or read about *The Waystones*.",
	CreatedBy: royID,
}

// InitialWorldCampaignID is the game set in InitialWorld.
const InitialWorldCampaignID = sojournID

// CodexSeedPage is a demo page. Links are written as [[Title]], so pages are matched by title.
// InWorld false puts the page in the Unsorted folder. Revisions after the first are amendments.
type CodexSeedPage struct {
	Title    string
	Aliases  []string
	AuthorID string
	InWorld  bool
	Folder   []string // folder names from the world's top level down; empty = top level
	Body     string
	Amend    []CodexSeedAmend
}

type CodexSeedAmend struct {
	AuthorID string
	Note     string
	Body     string
}

var InitialCodexPages = []CodexSeedPage{
	{Title: "The Karrow Road", AuthorID: royID, InWorld: true, Folder: []string{"Places"},
		Body: "The only road north out of [[Karrow Ford]]. It follows the river for a day, then climbs towards the pass.\n\n" +
			"A [[The Toll House|toll house]] stands at the second bend, kept by [[Old Hesk]]. The road is marked every few miles by one of [[The Waystones]]; the third is the last that anyone trusts.\n\n" +
			"Travellers say the road has been quieter since [[Cloveswick]] burned."},
	{Title: "Old Hesk", Aliases: []string{"Hesk"}, AuthorID: royID, InWorld: true, Folder: []string{"People"},
		Body: "Toll-keeper at [[The Toll House]]. Takes two silver a head and answers one question for free.\n\n" +
			"He says [[The Third Waystone]] has been warm since midsummer.",
		Amend: []CodexSeedAmend{{AuthorID: adamID, Note: "Added his past",
			Body: "Toll-keeper at [[The Toll House]]. Takes two silver a head and answers one question for free.\n\n" +
				"He says [[The Third Waystone]] has been warm since midsummer.\n\n" +
				"Before the toll he ferried people across at [[Karrow Ford]] with the [[Ferrymen of the Ford]]. He doesn't say why he stopped."}}},
	{Title: "Karrow Ford", AuthorID: adamID, InWorld: true, Folder: []string{"Places", "Settlements"},
		Body: "A river crossing and a market of about forty families. The [[Ferrymen of the Ford]] run the boats when the water is too high to wade.\n\n" +
			"The start of [[The Karrow Road]]."},
	{Title: "Ferrymen of the Ford", AuthorID: adamID, InWorld: true, Folder: []string{"Factions"},
		Body: "A loose guild of boatmen. They know every shallow on the river and gossip about everyone who crosses.\n\nTheir old leader was [[Old Hesk]]."},
	{Title: "The Waystones", AuthorID: testyID, InWorld: true, Folder: []string{"Places", "Landmarks", "Waystones"},
		Body: "Standing stones set along [[The Karrow Road]], one a day's walk from the next. They glow faintly at night and keep wolves away.\n\n" +
			"Nobody remembers who raised them. See [[The Third Waystone]], and the stories about [[The Ember Choir]]."},
	{Title: "The Third Waystone", AuthorID: testyID, InWorld: true, Folder: []string{"Places", "Landmarks", "Waystones"},
		Body: "The stone at the third bend of [[The Karrow Road]]. It went dark in the autumn and is **cold** to the touch.\n\n" +
			"[[Old Hesk]] swears it was warm all summer."},
	{Title: "The Toll House", AuthorID: royID, InWorld: true, Folder: []string{"Places", "Landmarks"},
		Body: "A squat stone house with a lamp that is never lit. [[Old Hesk]] lives here.\n\nIt marks the end of the safe stretch of [[The Karrow Road]]."},
	{Title: "Cloveswick", AuthorID: simonID, InWorld: true, Folder: []string{"Places", "Settlements"},
		Body: "A village that burned a few weeks ago. The ash smelled of cloves.\n\nOne survivor, a boy who only says *up, up, up*, was taken in at [[Karrow Ford]]."},
	{Title: "Saltmarsh Reach", AuthorID: simonID, InWorld: true, Folder: []string{"Places", "Settlements"},
		Body: "Brackish flats to the east of [[Karrow Ford]]. [[The Marsh Wardens]] patrol it."},
	{Title: "The Marsh Wardens", AuthorID: simonID, InWorld: true, Folder: []string{"Factions"},
		Body: "Lantern-bearers who keep the causeways of [[Saltmarsh Reach]] open. They owe [[Old Hesk]] a favour."},
	{Title: "Lamplighters' Guild", AuthorID: testyID, InWorld: true, Folder: []string{"Factions"},
		Body: "Keepers of the lamps along the great roads. Their charter is older than the [[The Waystones|waystones]], they say."},
	// Not in the world: sits in the Unsorted folder.
	{Title: "Wandering Merchants", AuthorID: adamID, InWorld: false,
		Body: "Peddlers who go wherever the roads are open. Not yet placed in any world."},
}

// InitialMapPage is the demo map. Pins name pages by title; "Hollow Tor" has no page yet.
var InitialMapPage = CodexSeedPage{Title: "Map of the Karrow", AuthorID: royID, InWorld: true,
	Body: "The road from [[Karrow Ford]] north to the pass, as the toll-keeper drew it."}

type CodexSeedPin struct {
	X, Y   float64
	Target string
}

// InitialSettlementMap is a small map on an ordinary page, so a pin to "Karrow Ford" shows
// "Has a map" in the panel. It reuses the demo image.
const InitialSettlementMapPage = "Karrow Ford"

var InitialSettlementMapPins = []CodexSeedPin{
	{X: 0.15, Y: 0.72, Target: "Ferrymen of the Ford"},
	{X: 0.45, Y: 0.5, Target: "The Toll House"},
}

var InitialMapPins = []CodexSeedPin{
	{X: 0.12, Y: 0.78, Target: "Karrow Ford"},
	{X: 0.42, Y: 0.55, Target: "The Toll House"},
	{X: 0.64, Y: 0.34, Target: "The Third Waystone"},
	{X: 0.84, Y: 0.18, Target: "Hollow Tor"},
}
