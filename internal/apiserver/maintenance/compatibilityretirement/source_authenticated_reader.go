package retirement

import "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"

// Only the coordinator's own post-authentication consumption readers use this
// observer. Public readers, every epoch's initial four-copy authentication and
// origin/index readers retain their independent SourceIdentityTracker.
// It preserves per-stream original-ID conflict errors, without retaining a
// second EventType/SourceRef for every authenticated row. It is not a proof of
// order, EOF, source coverage, business closure or retirement authorization.
type authenticatedSourceObserver struct {
	owner    *HistoricalCoordinator
	auth     *VerifiedSourceCopies
	object   uint8
	expected SourceCopyExpectation
	receipt  SourceCopyReceipt
	seen     map[string]struct{}
}

func (c *HistoricalCoordinator) newAuthenticatedObserver(object uint8) (*authenticatedSourceObserver, error) {
	if c == nil || c.authenticated == nil || !c.authenticated.complete || c.authenticated.entries != uint64(len(c.authenticated.rows)) || (object != 0 && object != 3) || sourceReaderAbsent(c.seekers[object]) {
		return nil, ErrSourceAuthentication
	}
	// Completeness is inherited only from the actual opaque four-copy builder;
	// all original expectations must still agree with its sealed receipts.
	names := [4]string{"domain_event_outbox", AIBridgeCommandSource, AILegacyCommandSource, "domain_event_outbox"}
	for i, copy := range c.copies {
		engine, protocol := "mysql", SQLSourceProtocol
		if i == 3 {
			engine, protocol = "mongodb", MongoSourceProtocol
		}
		e, r := copy.Expected, c.authenticated.receipts[i]
		if e.Boundary.Database != engine || e.Boundary.Name != names[i] || r.Protocol != protocol || !r.Complete || r.BusinessClosureVerified || r.DropReady || r.Records != e.Records || r.Bytes != e.Bytes || r.DataHash != e.DataHash {
			return nil, ErrSourceAuthentication
		}
	}
	o := &authenticatedSourceObserver{owner: c, auth: c.authenticated, object: object, expected: c.copies[object].Expected, receipt: c.authenticated.receipts[object]}
	if err := o.intact(); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *authenticatedSourceObserver) intact() error {
	if o == nil || o.owner == nil || o.auth == nil || o.owner.authenticated != o.auth || !o.auth.complete || (o.object != 0 && o.object != 3) || o.expected != o.owner.copies[o.object].Expected || o.receipt != o.auth.receipts[o.object] {
		return ErrSourceAuthentication
	}
	engine := "mysql"
	if o.object == 3 {
		engine = "mongodb"
	}
	if validExpectation(o.expected, engine) != nil {
		return ErrSourceAuthentication
	}
	return nil
}

func (o *authenticatedSourceObserver) observe(event *DecodedSourceEvent) error {
	if event == nil || !identifier(event.EventID, 64) || !evidence.ValidSHA256(event.Source.Digest.SHA256) {
		return ErrSourceIdentity
	}
	if err := o.intact(); err != nil {
		return err
	}
	// The native decoder has already rejected repeated/reordered PKs. A second
	// occurrence of this original ID therefore has a different source reference,
	// which the public tracker also reports as ErrSourceDigest. No editable DTO
	// or standalone tracker is admitted through this private observer.
	if _, exists := o.seen[event.EventID]; exists {
		return ErrSourceDigest
	}
	key, err := sourceAuthKey(event.Source.Database, event.Source.Object, event.Source.PrimaryKeySHA256)
	if err != nil {
		return err
	}
	if key.object != o.object {
		return ErrSourceAuthentication
	}
	row, exists := o.auth.rows[key]
	if !exists {
		return ErrSourceAuthentication
	}
	facts, err := privateFactsSHA(event)
	if err != nil {
		return err
	}
	if row.facts != facts {
		return ErrSourceAuthentication
	}
	if len(o.seen) >= MaxSourceRecords {
		return ErrSourceBounds
	}
	if o.seen == nil {
		o.seen = make(map[string]struct{})
	}
	o.seen[event.EventID] = struct{}{}
	return nil
}

func (c *HistoricalCoordinator) newAuthenticatedSQLSourceReader() (*SQLSourceReader, error) {
	o, err := c.newAuthenticatedObserver(0)
	if err != nil {
		return nil, err
	}
	r, err := NewSQLSourceReader(c.seekers[0], o.expected)
	if err != nil {
		return nil, err
	}
	r.acc.authenticated = o
	return r, nil
}

func (c *HistoricalCoordinator) newAuthenticatedMongoSourceReader() (*MongoSourceReader, error) {
	o, err := c.newAuthenticatedObserver(3)
	if err != nil {
		return nil, err
	}
	r, err := NewMongoSourceReader(c.seekers[3], o.expected)
	if err != nil {
		return nil, err
	}
	r.acc.authenticated = o
	return r, nil
}
