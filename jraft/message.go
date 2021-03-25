package jraft

type MsgType uint64

const (
	MsgAppend = iota
	MsgAppendResp
	MsgVote
	MsgVoteResp
	MsgHeartBeat
	MsgHeartBeatResp
)

//RPC发送消息
type Message struct {
	Type    MsgType //消息类型
	To      int     //to id
	From    int     //from id
	Term    int     //任期
	LogTerm int
	Index   int
	Entries []entry
	Commit  int
	Reject  bool   //是否拒绝
	Reason  string //拒绝理由
}
