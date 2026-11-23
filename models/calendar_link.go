package models

// CalendarLink is a user's calendar feed link, without its secret token. LastReadAt is when a
// calendar app last read it, '' if never (or not since the link was regenerated).
type CalendarLink struct {
	CreatedAt  string
	LastReadAt string
}
