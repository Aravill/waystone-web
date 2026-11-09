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
	Kind     string
	Title    string
	Aliases  []string
	AuthorID string
	InWorld  bool
	Body     string
	Amend    []CodexSeedAmend
}

type CodexSeedAmend struct {
	AuthorID string
	Note     string
	Body     string
}

var InitialCodexPages = []CodexSeedPage{
	{Kind: models.PageLore, Title: "The Karrow Road", AuthorID: royID, InWorld: true,
		Body: "The only road north out of [[Karrow Ford]]. It follows the river for a day, then climbs towards the pass.\n\n" +
			"A [[The Toll House|toll house]] stands at the second bend, kept by [[Old Hesk]]. The road is marked every few miles by one of [[The Waystones]]; the third is the last that anyone trusts.\n\n" +
			"Travellers say the road has been quieter since [[Cloveswick]] burned."},
	{Kind: models.PageLore, Title: "Old Hesk", Aliases: []string{"Hesk"}, AuthorID: royID, InWorld: true,
		Body: "Toll-keeper at [[The Toll House]]. Takes two silver a head and answers one question for free.\n\n" +
			"He says [[The Third Waystone]] has been warm since midsummer.",
		Amend: []CodexSeedAmend{{AuthorID: adamID, Note: "Added his past",
			Body: "Toll-keeper at [[The Toll House]]. Takes two silver a head and answers one question for free.\n\n" +
				"He says [[The Third Waystone]] has been warm since midsummer.\n\n" +
				"Before the toll he ferried people across at [[Karrow Ford]] with the [[Ferrymen of the Ford]]. He doesn't say why he stopped."}}},
	{Kind: models.PageLore, Title: "Karrow Ford", AuthorID: adamID, InWorld: true,
		Body: "A river crossing and a market of about forty families. The [[Ferrymen of the Ford]] run the boats when the water is too high to wade.\n\n" +
			"The start of [[The Karrow Road]]."},
	{Kind: models.PageLore, Title: "Ferrymen of the Ford", AuthorID: adamID, InWorld: true,
		Body: "A loose guild of boatmen. They know every shallow on the river and gossip about everyone who crosses.\n\nTheir old leader was [[Old Hesk]]."},
	{Kind: models.PageLore, Title: "The Waystones", AuthorID: testyID, InWorld: true,
		Body: "Standing stones set along [[The Karrow Road]], one a day's walk from the next. They glow faintly at night and keep wolves away.\n\n" +
			"Nobody remembers who raised them. See [[The Third Waystone]], and the stories about [[The Ember Choir]]."},
	{Kind: models.PageLore, Title: "The Third Waystone", AuthorID: testyID, InWorld: true,
		Body: "The stone at the third bend of [[The Karrow Road]]. It went dark in the autumn and is **cold** to the touch.\n\n" +
			"[[Old Hesk]] swears it was warm all summer."},
	{Kind: models.PageLore, Title: "The Toll House", AuthorID: royID, InWorld: true,
		Body: "A squat stone house with a lamp that is never lit. [[Old Hesk]] lives here.\n\nIt marks the end of the safe stretch of [[The Karrow Road]]."},
	{Kind: models.PageLore, Title: "Cloveswick", AuthorID: simonID, InWorld: true,
		Body: "A village that burned a few weeks ago. The ash smelled of cloves.\n\nOne survivor, a boy who only says *up, up, up*, was taken in at [[Karrow Ford]]."},
	{Kind: models.PageLore, Title: "Saltmarsh Reach", AuthorID: simonID, InWorld: true,
		Body: "Brackish flats to the east of [[Karrow Ford]]. [[The Marsh Wardens]] patrol it."},
	{Kind: models.PageLore, Title: "The Marsh Wardens", AuthorID: simonID, InWorld: true,
		Body: "Lantern-bearers who keep the causeways of [[Saltmarsh Reach]] open. They owe [[Old Hesk]] a favour."},
	{Kind: models.PageLore, Title: "Lamplighters' Guild", AuthorID: testyID, InWorld: true,
		Body: "Keepers of the lamps along the great roads. Their charter is older than the [[The Waystones|waystones]], they say."},
	// Not in the world: sits in the Unsorted folder.
	{Kind: models.PageLore, Title: "Wandering Merchants", AuthorID: adamID, InWorld: false,
		Body: "Peddlers who go wherever the roads are open. Not yet placed in any world."},
}

// InitialMapPage is the demo map. Pins name pages by title; "Hollow Tor" has no page yet.
var InitialMapPage = CodexSeedPage{Kind: models.PageMap, Title: "Map of the Karrow", AuthorID: royID, InWorld: true,
	Body: "The road from [[Karrow Ford]] north to the pass, as the toll-keeper drew it."}

type CodexSeedPin struct {
	X, Y   float64
	Target string
}

var InitialMapPins = []CodexSeedPin{
	{X: 0.12, Y: 0.78, Target: "Karrow Ford"},
	{X: 0.42, Y: 0.55, Target: "The Toll House"},
	{X: 0.64, Y: 0.34, Target: "The Third Waystone"},
	{X: 0.84, Y: 0.18, Target: "Hollow Tor"},
}
