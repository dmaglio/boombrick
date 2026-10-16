package media

// Cover names an album's art: the album artist and title, and one of the
// album's files, whose tags and folder are where its picture is looked for.
type Cover struct {
	Artist, Album, Path string
}

// Key identifies the album; covers with the same key share their art. It is
// empty when there is no album to name.
func (c Cover) Key() string {
	if c.Album == "" {
		return ""
	}
	return c.Artist + "\x1f" + c.Album
}
