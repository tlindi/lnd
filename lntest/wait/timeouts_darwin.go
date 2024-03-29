//go:build darwin && !kvdb_etcd && !kvdb_postgres
// +build darwin,!kvdb_etcd,!kvdb_postgres

package wait

import "time"

const (
	// MinerMempoolTimeout is the max time we will wait for a transaction
	// to propagate to the mining node's mempool.
	MinerMempoolTimeout = time.Second * 5

	// ChannelOpenTimeout is the max time we will wait before a channel to
	// be considered opened.
	ChannelOpenTimeout = time.Second * 30

	// ChannelCloseTimeout is the max time we will wait before a channel is
	// considered closed.
	ChannelCloseTimeout = time.Second * 60

	// DefaultTimeout is a timeout that will be used for various wait
	// scenarios where no custom timeout value is defined.
	DefaultTimeout = time.Second * 5

	// AsyncBenchmarkTimeout is the timeout used when running the async
	// payments benchmark. This timeout takes considerably longer on darwin
	// after go1.12 corrected its use of fsync.
	AsyncBenchmarkTimeout = time.Minute * 5

	// NodeStartTimeout is the timeout value when waiting for a node to
	// become fully started.
	NodeStartTimeout = time.Minute * 2

	// SqliteBusyTimeout is the maximum time that a call to the sqlite db
	// will wait for the connection to become available.
	SqliteBusyTimeout = time.Second * 10

	// PaymentTimeout is the timeout used when sending payments.
	PaymentTimeout = time.Second * 60
)
