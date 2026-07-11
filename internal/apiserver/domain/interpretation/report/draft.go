package report

// Draft is the immutable content assembled by Interpretation before a
// Generation/Run commits it as an Artifact. It deliberately carries neither
// lifecycle state nor persistence identifiers.
type Draft struct {
	content Content
}

func NewDraft(content Content) *Draft {
	return &Draft{content: cloneContent(content)}
}

func (d *Draft) Content() Content {
	if d == nil {
		return Content{}
	}
	return cloneContent(d.content)
}
