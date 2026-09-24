package agentstate

type State struct {
	ServerID        int64
	AppliedRevision int64
	TrafficBatchSeq int64
	DeviceBatchSeq  int64
	VisitBatchSeq   int64
}
