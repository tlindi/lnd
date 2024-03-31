package channeldb

import (
	"bytes"
	"errors"
	"io"
	"math"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/fn"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// OutputIndexEmpty is used when the output index doesn't exist.
	OutputIndexEmpty = math.MaxUint16

	// A set of tlv type definitions used to serialize the body of
	// revocation logs to the database.
	//
	// NOTE: A migration should be added whenever this list changes.
	revLogOurOutputIndexType   tlv.Type = 0
	revLogTheirOutputIndexType tlv.Type = 1
	revLogCommitTxHashType     tlv.Type = 2
	revLogOurBalanceType       tlv.Type = 3
	revLogTheirBalanceType     tlv.Type = 4
)

var (
	// revocationLogBucketDeprecated is dedicated for storing the necessary
	// delta state between channel updates required to re-construct a past
	// state in order to punish a counterparty attempting a non-cooperative
	// channel closure. This key should be accessed from within the
	// sub-bucket of a target channel, identified by its channel point.
	//
	// Deprecated: This bucket is kept for read-only in case the user
	// choose not to migrate the old data.
	revocationLogBucketDeprecated = []byte("revocation-log-key")

	// revocationLogBucket is a sub-bucket under openChannelBucket. This
	// sub-bucket is dedicated for storing the minimal info required to
	// re-construct a past state in order to punish a counterparty
	// attempting a non-cooperative channel closure.
	revocationLogBucket = []byte("revocation-log")

	// ErrLogEntryNotFound is returned when we cannot find a log entry at
	// the height requested in the revocation log.
	ErrLogEntryNotFound = errors.New("log entry not found")

	// ErrOutputIndexTooBig is returned when the output index is greater
	// than uint16.
	ErrOutputIndexTooBig = errors.New("output index is over uint16")
)

// SparseRHash is a type alias for a 32 byte array, which when serialized is
// able to save some space by not including an empty payment hash on disk.
type SparsePayHash [32]byte

// NewSparsePayHash creates a new SparsePayHash from a 32 byte array.
func NewSparsePayHash(rHash [32]byte) SparsePayHash {
	return SparsePayHash(rHash)
}

// Record returns a tlv record for the SparsePayHash.
func (s *SparsePayHash) Record() tlv.Record {
	// We use a zero for the type here, as this'll be used along with the
	// RecordT type.
	return tlv.MakeDynamicRecord(
		0, s, s.hashLen,
		sparseHashEncoder, sparseHashDecoder,
	)
}

// hashLen is used by MakeDynamicRecord to return the size of the RHash.
//
// NOTE: for zero hash, we return a length 0.
func (s *SparsePayHash) hashLen() uint64 {
	if bytes.Equal(s[:], lntypes.ZeroHash[:]) {
		return 0
	}

	return 32
}

// sparseHashEncoder is the customized encoder which skips encoding the empty
// hash.
func sparseHashEncoder(w io.Writer, val interface{}, buf *[8]byte) error {
	v, ok := val.(*SparsePayHash)
	if !ok {
		return tlv.NewTypeForEncodingErr(val, "SparsePayHash")
	}

	// If the value is an empty hash, we will skip encoding it.
	if bytes.Equal(v[:], lntypes.ZeroHash[:]) {
		return nil
	}

	vArray := (*[32]byte)(v)

	return tlv.EBytes32(w, vArray, buf)
}

// sparseHashDecoder is the customized decoder which skips decoding the empty
// hash.
func sparseHashDecoder(r io.Reader, val interface{}, buf *[8]byte,
	l uint64) error {

	v, ok := val.(*SparsePayHash)
	if !ok {
		return tlv.NewTypeForEncodingErr(val, "SparsePayHash")
	}

	// If the length is zero, we will skip encoding the empty hash.
	if l == 0 {
		return nil
	}

	vArray := (*[32]byte)(v)

	if err := tlv.DBytes32(r, vArray, buf, 32); err != nil {
		return err
	}

	vHash := SparsePayHash(*vArray)

	v = &vHash

	return nil
}

// HTLCEntry specifies the minimal info needed to be stored on disk for ALL the
// historical HTLCs, which is useful for constructing RevocationLog when a
// breach is detected.
// The actual size of each HTLCEntry varies based on its RHash and Amt(sat),
// summarized as follows,
//
//	| RHash empty | Amt<=252 | Amt<=65,535 | Amt<=4,294,967,295 | otherwise |
//	|:-----------:|:--------:|:-----------:|:------------------:|:---------:|
//	|     true    |    19    |      21     |         23         |     26    |
//	|     false   |    51    |      53     |         55         |     58    |
//
// So the size varies from 19 bytes to 58 bytes, where most likely to be 23 or
// 55 bytes.
//
// NOTE: all the fields saved to disk use the primitive go types so they can be
// made into tlv records without further conversion.
type HTLCEntry struct {
	// RHash is the payment hash of the HTLC.
	RHash tlv.RecordT[tlv.TlvType0, SparsePayHash]

	// RefundTimeout is the absolute timeout on the HTLC that the sender
	// must wait before reclaiming the funds in limbo.
	RefundTimeout tlv.RecordT[tlv.TlvType1, uint32]

	// OutputIndex is the output index for this particular HTLC output
	// within the commitment transaction.
	//
	// NOTE: we use uint16 instead of int32 here to save us 2 bytes, which
	// gives us a max number of HTLCs of 65K.
	OutputIndex tlv.RecordT[tlv.TlvType2, uint16]

	// Incoming denotes whether we're the receiver or the sender of this
	// HTLC.
	//
	// NOTE: this field is the memory representation of the field
	// incomingUint.
	Incoming tlv.RecordT[tlv.TlvType3, bool]

	// Amt is the amount of satoshis this HTLC escrows.
	//
	// NOTE: this field is the memory representation of the field amtUint.
	Amt tlv.RecordT[tlv.TlvType4, tlv.BigSizeT[btcutil.Amount]]
}

// toTlvStream converts an HTLCEntry record into a tlv representation.
func (h *HTLCEntry) toTlvStream() (*tlv.Stream, error) {
	return tlv.NewStream(
		h.RHash.Record(),
		h.RefundTimeout.Record(),
		h.OutputIndex.Record(),
		h.Incoming.Record(),
		h.Amt.Record(),
	)
}

// NewHTLCEntryFromHTLC creates a new HTLCEntry from an HTLC.
func NewHTLCEntryFromHTLC(htlc HTLC) *HTLCEntry {
	return &HTLCEntry{
		RHash: tlv.NewRecordT[tlv.TlvType0, SparsePayHash](
			NewSparsePayHash(htlc.RHash),
		),
		RefundTimeout: tlv.NewPrimitiveRecord[tlv.TlvType1, uint32](
			htlc.RefundTimeout,
		),
		OutputIndex: tlv.NewPrimitiveRecord[tlv.TlvType2, uint16](
			uint16(htlc.OutputIndex),
		),
		Incoming: tlv.NewPrimitiveRecord[tlv.TlvType3, bool](
			htlc.Incoming,
		),
		Amt: tlv.NewRecordT[tlv.TlvType4, tlv.BigSizeT[btcutil.Amount]](
			tlv.NewBigSizeT(htlc.Amt.ToSatoshis()),
		),
	}
}

// RevocationLog stores the info needed to construct a breach retribution. Its
// fields can be viewed as a subset of a ChannelCommitment's. In the database,
// all historical versions of the RevocationLog are saved using the
// CommitHeight as the key.
type RevocationLog struct {
	// OurOutputIndex specifies our output index in this commitment. In a
	// remote commitment transaction, this is the to remote output index.
	OurOutputIndex uint16

	// TheirOutputIndex specifies their output index in this commitment. In
	// a remote commitment transaction, this is the to local output index.
	TheirOutputIndex uint16

	// CommitTxHash is the hash of the latest version of the commitment
	// state, broadcast able by us.
	CommitTxHash [32]byte

	// HTLCEntries is the set of HTLCEntry's that are pending at this
	// particular commitment height.
	HTLCEntries []*HTLCEntry

	// OurBalance is the current available balance within the channel
	// directly spendable by us. In other words, it is the value of the
	// to_remote output on the remote parties' commitment transaction.
	//
	// NOTE: this is a pointer so that it is clear if the value is zero or
	// nil. Since migration 30 of the channeldb initially did not include
	// this field, it could be the case that the field is not present for
	// all revocation logs.
	OurBalance *lnwire.MilliSatoshi

	// TheirBalance is the current available balance within the channel
	// directly spendable by the remote node. In other words, it is the
	// value of the to_local output on the remote parties' commitment.
	//
	// NOTE: this is a pointer so that it is clear if the value is zero or
	// nil. Since migration 30 of the channeldb initially did not include
	// this field, it could be the case that the field is not present for
	// all revocation logs.
	TheirBalance *lnwire.MilliSatoshi
}

// putRevocationLog uses the fields `CommitTx` and `Htlcs` from a
// ChannelCommitment to construct a revocation log entry and saves them to
// disk. It also saves our output index and their output index, which are
// useful when creating breach retribution.
func putRevocationLog(bucket kvdb.RwBucket, commit *ChannelCommitment,
	ourOutputIndex, theirOutputIndex uint32, noAmtData bool) error {

	// Sanity check that the output indexes can be safely converted.
	if ourOutputIndex > math.MaxUint16 {
		return ErrOutputIndexTooBig
	}
	if theirOutputIndex > math.MaxUint16 {
		return ErrOutputIndexTooBig
	}

	rl := &RevocationLog{
		OurOutputIndex:   uint16(ourOutputIndex),
		TheirOutputIndex: uint16(theirOutputIndex),
		CommitTxHash:     commit.CommitTx.TxHash(),
		HTLCEntries:      make([]*HTLCEntry, 0, len(commit.Htlcs)),
	}

	if !noAmtData {
		rl.OurBalance = &commit.LocalBalance
		rl.TheirBalance = &commit.RemoteBalance
	}

	for _, htlc := range commit.Htlcs {
		// Skip dust HTLCs.
		if htlc.OutputIndex < 0 {
			continue
		}

		// Sanity check that the output indexes can be safely
		// converted.
		if htlc.OutputIndex > math.MaxUint16 {
			return ErrOutputIndexTooBig
		}

		entry := NewHTLCEntryFromHTLC(htlc)
		rl.HTLCEntries = append(rl.HTLCEntries, entry)
	}

	var b bytes.Buffer
	err := serializeRevocationLog(&b, rl)
	if err != nil {
		return err
	}

	logEntrykey := makeLogKey(commit.CommitHeight)
	return bucket.Put(logEntrykey[:], b.Bytes())
}

// fetchRevocationLog queries the revocation log bucket to find an log entry.
// Return an error if not found.
func fetchRevocationLog(log kvdb.RBucket,
	updateNum uint64) (RevocationLog, error) {

	logEntrykey := makeLogKey(updateNum)
	commitBytes := log.Get(logEntrykey[:])
	if commitBytes == nil {
		return RevocationLog{}, ErrLogEntryNotFound
	}

	commitReader := bytes.NewReader(commitBytes)

	return deserializeRevocationLog(commitReader)
}

// serializeRevocationLog serializes a RevocationLog record based on tlv
// format.
func serializeRevocationLog(w io.Writer, rl *RevocationLog) error {
	// Add the tlv records for all non-optional fields.
	records := []tlv.Record{
		tlv.MakePrimitiveRecord(
			revLogOurOutputIndexType, &rl.OurOutputIndex,
		),
		tlv.MakePrimitiveRecord(
			revLogTheirOutputIndexType, &rl.TheirOutputIndex,
		),
		tlv.MakePrimitiveRecord(
			revLogCommitTxHashType, &rl.CommitTxHash,
		),
	}

	// Now we add any optional fields that are non-nil.
	if rl.OurBalance != nil {
		lb := uint64(*rl.OurBalance)
		records = append(records, tlv.MakeBigSizeRecord(
			revLogOurBalanceType, &lb,
		))
	}

	if rl.TheirBalance != nil {
		rb := uint64(*rl.TheirBalance)
		records = append(records, tlv.MakeBigSizeRecord(
			revLogTheirBalanceType, &rb,
		))
	}

	// Create the tlv stream.
	tlvStream, err := tlv.NewStream(records...)
	if err != nil {
		return err
	}

	// Write the tlv stream.
	if err := writeTlvStream(w, tlvStream); err != nil {
		return err
	}

	// Write the HTLCs.
	return serializeHTLCEntries(w, rl.HTLCEntries)
}

// serializeHTLCEntries serializes a list of HTLCEntry records based on tlv
// format.
func serializeHTLCEntries(w io.Writer, htlcs []*HTLCEntry) error {
	for _, htlc := range htlcs {
		// Create the tlv stream.
		tlvStream, err := htlc.toTlvStream()
		if err != nil {
			return err
		}

		// Write the tlv stream.
		if err := writeTlvStream(w, tlvStream); err != nil {
			return err
		}
	}

	return nil
}

// deserializeRevocationLog deserializes a RevocationLog based on tlv format.
func deserializeRevocationLog(r io.Reader) (RevocationLog, error) {
	var (
		rl           RevocationLog
		ourBalance   uint64
		theirBalance uint64
	)

	// Create the tlv stream.
	tlvStream, err := tlv.NewStream(
		tlv.MakePrimitiveRecord(
			revLogOurOutputIndexType, &rl.OurOutputIndex,
		),
		tlv.MakePrimitiveRecord(
			revLogTheirOutputIndexType, &rl.TheirOutputIndex,
		),
		tlv.MakePrimitiveRecord(
			revLogCommitTxHashType, &rl.CommitTxHash,
		),
		tlv.MakeBigSizeRecord(revLogOurBalanceType, &ourBalance),
		tlv.MakeBigSizeRecord(
			revLogTheirBalanceType, &theirBalance,
		),
	)
	if err != nil {
		return rl, err
	}

	// Read the tlv stream.
	parsedTypes, err := readTlvStream(r, tlvStream)
	if err != nil {
		return rl, err
	}

	if t, ok := parsedTypes[revLogOurBalanceType]; ok && t == nil {
		lb := lnwire.MilliSatoshi(ourBalance)
		rl.OurBalance = &lb
	}

	if t, ok := parsedTypes[revLogTheirBalanceType]; ok && t == nil {
		rb := lnwire.MilliSatoshi(theirBalance)
		rl.TheirBalance = &rb
	}

	// Read the HTLC entries.
	rl.HTLCEntries, err = deserializeHTLCEntries(r)

	return rl, err
}

// deserializeHTLCEntries deserializes a list of HTLC entries based on tlv
// format.
func deserializeHTLCEntries(r io.Reader) ([]*HTLCEntry, error) {
	var htlcs []*HTLCEntry

	for {
		var htlc HTLCEntry

		// Create the tlv stream.
		tlvStream, err := htlc.toTlvStream()
		if err != nil {
			return nil, err
		}

		// Read the HTLC entry.
		if _, err := readTlvStream(r, tlvStream); err != nil {
			// We've reached the end when hitting an EOF.
			if err == io.ErrUnexpectedEOF {
				break
			}
			return nil, err
		}

		// Append the entry.
		htlcs = append(htlcs, &htlc)
	}

	return htlcs, nil
}

// writeTlvStream is a helper function that encodes the tlv stream into the
// writer.
func writeTlvStream(w io.Writer, s *tlv.Stream) error {
	var b bytes.Buffer
	if err := s.Encode(&b); err != nil {
		return err
	}

	// Write the stream's length as a varint.
	err := tlv.WriteVarInt(w, uint64(b.Len()), &[8]byte{})
	if err != nil {
		return err
	}

	if _, err = w.Write(b.Bytes()); err != nil {
		return err
	}

	return nil
}

// readTlvStream is a helper function that decodes the tlv stream from the
// reader.
func readTlvStream(r io.Reader, s *tlv.Stream) (tlv.TypeMap, error) {
	var bodyLen uint64

	// Read the stream's length.
	bodyLen, err := tlv.ReadVarInt(r, &[8]byte{})
	switch {
	// We'll convert any EOFs to ErrUnexpectedEOF, since this results in an
	// invalid record.
	case err == io.EOF:
		return nil, io.ErrUnexpectedEOF

	// Other unexpected errors.
	case err != nil:
		return nil, err
	}

	// TODO(yy): add overflow check.
	lr := io.LimitReader(r, int64(bodyLen))

	return s.DecodeWithParsedTypes(lr)
}

// fetchOldRevocationLog finds the revocation log from the deprecated
// sub-bucket.
func fetchOldRevocationLog(log kvdb.RBucket,
	updateNum uint64) (ChannelCommitment, error) {

	logEntrykey := makeLogKey(updateNum)
	commitBytes := log.Get(logEntrykey[:])
	if commitBytes == nil {
		return ChannelCommitment{}, ErrLogEntryNotFound
	}

	commitReader := bytes.NewReader(commitBytes)
	return deserializeChanCommit(commitReader)
}

// fetchRevocationLogCompatible finds the revocation log from both the
// revocationLogBucket and revocationLogBucketDeprecated for compatibility
// concern. It returns three values,
//   - RevocationLog, if this is non-nil, it means we've found the log in the
//     new bucket.
//   - ChannelCommitment, if this is non-nil, it means we've found the log in the
//     old bucket.
//   - error, this can happen if the log cannot be found in neither buckets.
func fetchRevocationLogCompatible(chanBucket kvdb.RBucket,
	updateNum uint64) (*RevocationLog, *ChannelCommitment, error) {

	// Look into the new bucket first.
	logBucket := chanBucket.NestedReadBucket(revocationLogBucket)
	if logBucket != nil {
		rl, err := fetchRevocationLog(logBucket, updateNum)
		// We've found the record, no need to visit the old bucket.
		if err == nil {
			return &rl, nil, nil
		}

		// Return the error if it doesn't say the log cannot be found.
		if err != ErrLogEntryNotFound {
			return nil, nil, err
		}
	}

	// Otherwise, look into the old bucket and try to find the log there.
	oldBucket := chanBucket.NestedReadBucket(revocationLogBucketDeprecated)
	if oldBucket != nil {
		c, err := fetchOldRevocationLog(oldBucket, updateNum)
		if err != nil {
			return nil, nil, err
		}

		// Found an old record and return it.
		return nil, &c, nil
	}

	// If both the buckets are nil, then the sub-buckets haven't been
	// created yet.
	if logBucket == nil && oldBucket == nil {
		return nil, nil, ErrNoPastDeltas
	}

	// Otherwise, we've tried to query the new bucket but the log cannot be
	// found.
	return nil, nil, ErrLogEntryNotFound
}

// fetchLogBucket returns a read bucket by visiting both the new and the old
// bucket.
func fetchLogBucket(chanBucket kvdb.RBucket) (kvdb.RBucket, error) {
	logBucket := chanBucket.NestedReadBucket(revocationLogBucket)
	if logBucket == nil {
		logBucket = chanBucket.NestedReadBucket(
			revocationLogBucketDeprecated,
		)
		if logBucket == nil {
			return nil, ErrNoPastDeltas
		}
	}

	return logBucket, nil
}

// deleteLogBucket deletes the both the new and old revocation log buckets.
func deleteLogBucket(chanBucket kvdb.RwBucket) error {
	// Check if the bucket exists and delete it.
	logBucket := chanBucket.NestedReadWriteBucket(
		revocationLogBucket,
	)
	if logBucket != nil {
		err := chanBucket.DeleteNestedBucket(revocationLogBucket)
		if err != nil {
			return err
		}
	}

	// We also check whether the old revocation log bucket exists
	// and delete it if so.
	oldLogBucket := chanBucket.NestedReadWriteBucket(
		revocationLogBucketDeprecated,
	)
	if oldLogBucket != nil {
		err := chanBucket.DeleteNestedBucket(
			revocationLogBucketDeprecated,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
