//go:build !integration

package lncfg

// ProtocolOptions is a struct that we use to be able to test backwards
// compatibility of protocol additions, while defaulting to the latest within
// lnd, or to enable experimental protocol changes.
//
//nolint:lll
type ProtocolOptions struct {
	// LegacyProtocol is a sub-config that houses all the legacy protocol
	// options.  These are mostly used for integration tests as most modern
	// nodes should always run with them on by default.
	LegacyProtocol `group:"legacy" namespace:"legacy"`

	// ExperimentalProtocol is a sub-config that houses any experimental
	// protocol features that also require a build-tag to activate.
	ExperimentalProtocol

	// WumboChans should be set if we want to enable support for wumbo
	// (channels larger than 0.16 BTC) channels, which is the opposite of
	// mini.
	WumboChans bool `long:"wumbo-channels" description:"if set, then lnd will create and accept requests for channels larger chan 0.16 BTC"`

	// TaprootChans should be set if we want to enable support for the
	// experimental simple taproot chans commitment type.
	TaprootChans bool `long:"simple-taproot-chans" description:"if set, then lnd will create and accept requests for channels using the simple taproot commitment type"`

	// RbfCoopClose should be set if we want to signal that we support for
	// the new experimental RBF coop close feature.
	RbfCoopClose bool `long:"rbf-coop-close" description:"if set, then lnd will signal that it supports the new RBF based coop close protocol"`

	// NoAnchors should be set if we don't want to support opening or accepting
	// channels having the anchor commitment type.
	NoAnchors bool `long:"no-anchors" description:"disable support for anchor commitments"`

	// NoScriptEnforcedLease should be set if we don't want to support
	// opening or accepting channels having the script enforced commitment
	// type for leased channel.
	NoScriptEnforcedLease bool `long:"no-script-enforced-lease" description:"disable support for script enforced lease commitments"`

	// OptionScidAlias should be set if we want to signal the
	// option-scid-alias feature bit. This allows scid aliases and the
	// option-scid-alias channel-type.
	OptionScidAlias bool `long:"option-scid-alias" description:"enable support for option_scid_alias channels"`

	// OptionZeroConf should be set if we want to signal the zero-conf
	// feature bit.
	OptionZeroConf bool `long:"zero-conf" description:"enable support for zero-conf channels, must have option-scid-alias set also"`

	// NoOptionAnySegwit should be set to true if we don't want to use any
	// Taproot (and beyond) addresses for co-op closing.
	NoOptionAnySegwit bool `long:"no-any-segwit" description:"disallow using any segwit witness version as a co-op close address"`

	// NoTimestampQueryOption should be set to true if we don't want our
	// syncing peers to also send us the timestamps of announcement messages
	// when we send them a channel range query. Setting this to true will
	// also mean that we won't respond with timestamps if requested by our
	// peers.
	NoTimestampQueryOption bool `long:"no-timestamp-query-option" description:"do not query syncing peers for announcement timestamps and do not respond with timestamps if requested"`
}

// Wumbo returns true if lnd should permit the creation and acceptance of wumbo
// channels.
func (l *ProtocolOptions) Wumbo() bool {
	return l.WumboChans
}

// NoAnchorCommitments returns true if we have disabled support for the anchor
// commitment type.
func (l *ProtocolOptions) NoAnchorCommitments() bool {
	return l.NoAnchors
}

// NoScriptEnforcementLease returns true if we have disabled support for the
// script enforcement commitment type for leased channels.
func (l *ProtocolOptions) NoScriptEnforcementLease() bool {
	return l.NoScriptEnforcedLease
}

// ScidAlias returns true if we have enabled the option-scid-alias feature bit.
func (l *ProtocolOptions) ScidAlias() bool {
	return l.OptionScidAlias
}

// ZeroConf returns true if we have enabled the zero-conf feature bit.
func (l *ProtocolOptions) ZeroConf() bool {
	return l.OptionZeroConf
}

// NoAnySegwit returns true if we don't signal that we understand other newer
// segwit witness versions for co-op close addresses.
func (l *ProtocolOptions) NoAnySegwit() bool {
	return l.NoOptionAnySegwit
}

// NoTimestampsQuery returns true if we should not ask our syncing peers to also
// send us the timestamps of announcement messages when we send them a channel
// range query, and it also means that we will not respond with timestamps if
// requested by our peer.
func (l *ProtocolOptions) NoTimestampsQuery() bool {
	return l.NoTimestampQueryOption
}
