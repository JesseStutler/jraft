package jraft

import (
	"errors"
	"log"
	"math/rand"
	"time"
)

type stateType int
type stepFunc func(m Message) error

const (
	StateFollower stateType = iota
	StateCandidate
	StateLeader
)

const None = 0

//主要数据结构
type raft struct {
	id               int
	leaderid         int
	voteFor          int
	peers            map[int]string //各id和其端点
	Term             int            //任期
	State            stateType      //状态
	Log              *raftlog       //日志
	votes            map[int]bool   //收到了来自哪些节点的投票
	tick             func()         //计时器
	MsgChan          chan Message   //消息通道
	tickChan         chan struct{}  //触发定时器通道
	step             stepFunc       //在各阶段的处理函数
	electionElapsed  time.Duration
	heartbeatElapsed time.Duration
	//随机超时时间，防止出现投票分歧，值在[elctionTimeout,electionTimeout*2-1]之间
	randomElectionTimeout time.Duration
	electionTimeout       time.Duration
	heartbearTimeout      time.Duration
}

//配置信息
type config struct {
	peersNum         int           //有多少台state machine，默认5台
	electionTimeout  time.Duration //选举超时时间默认两分钟
	heartbearTimeout time.Duration //心跳超时时间默认30秒
}

//id和其端点
type peer struct {
	ID       int
	endpoint string
}

//初始化raft函数
func NewRaft(c *config) *raft {
	return &raft{
		peers: make(map[int]string, c.peersNum),
		Term:  0,             //任期从0开始
		State: StateFollower, //初始皆为Follower
		Log: &raftlog{
			entries: make([]entry, 0),
		},
		votes:                 make(map[int]bool, c.peersNum),
		MsgChan:               make(chan Message),
		electionTimeout:       c.electionTimeout,
		heartbearTimeout:      c.heartbearTimeout,
		randomElectionTimeout: time.Duration(Rand(int64(c.electionTimeout), int64(c.electionTimeout)*2-1)),
	}
}

func Run(c *config, peers []peer) {
	r := NewRaft(c)
	r.becomeFollower(0, None) //初始时没有leader
	//记录id和其端点
	for _, peer := range peers {
		r.peers[peer.ID] = peer.endpoint
	}
	r.tickChan <- struct{}{}
	go r.listen()
}

//生成区间内随机数
func Rand(min, max int64) int64 {
	return rand.Int63n(max-min) + min
}

//一直监听有无Message过来
func (r *raft) listen() {
	for {
		//Doing：看select是否还需要加选项
		select {
		case m := <-r.MsgChan:
			r.step(m)
		case <-r.tickChan:
			r.tick()
		}
	}
}

//quorum代表大多数
func (r *raft) quorum() int { return len(r.peers)/2 + 1 }

//追加条目RPC，空条目代表heartbeat
func (r *raft) AppendEntries(m Message) {

}

func (r *raft) RequestVote(id int, endpoint string) {
	r.sendMessage(Message{Type: MsgVote, From: r.id, To: id, Term: r.Term})
	//算自己的票是否获得了大多数，如果是大多数的话则变为leader

	var granted int = 1
	for _, voted := range r.votes {
		if voted == true {
			granted++
		}
	}

	if granted == r.quorum() {
		r.becomeLeader()
	}
}

func (r *raft) becomeFollower(term, leaderid int) {
	r.Term = term
	r.step = r.stepFollower
	r.State = StateFollower
	r.leaderid = leaderid
	r.tick = r.tickElection
	log.Printf("[INFO]%d become follower at term %d", r.id, r.Term)
}

func (r *raft) becomeCandidate() {
	r.Term++ //开始新一轮任期
	r.step = r.stepCandidate
	r.State = StateCandidate
	r.voteFor = r.id //给自己投一票
	log.Printf("[INFO]%d become candiate at term %d", r.id, r.Term)
	for id, endpoint := range r.peers {
		r.RequestVote(id, endpoint)
	}
}

func (r *raft) becomeLeader() {
	r.tick = r.tickHeartBeat
}

//Doing
//根据Message类型来决定操作
func (r *raft) stepFollower(m Message) error {
	switch m.Type {
	case MsgAppend:
		//TODO
		//判断是否可以追加条目
		r.Log.entries = append(r.Log.entries, m.Entries...)
		r.Log.index += len(m.Entries)
		r.sendMessage(Message{Type: MsgAppendResp, From: r.id, To: m.From, Term: r.Term})
	case MsgVote:
		//TODO
		if r.voteFor != 0 {
			r.sendMessage(Message{})
		} else {
			r.voteFor = m.From
		}
	case MsgHeartBeat:
		r.leaderid = m.From
		r.electionElapsed = 0
		r.sendMessage(Message{Type: MsgHeartBeatResp, From: r.id, To: m.From, Term: r.Term})
	//无匹配
	default:
		return errors.New("no MsgType match")
	}
	return nil
}

func (r *raft) stepCandidate(m Message) error {
	switch m.Type {
	case MsgHeartBeat:
		//Candidate收到来自leader的心跳，其Term必须是最新的，否则拒绝并保持Candiate状态
		if r.Term > m.Term {
			r.sendMessage(Message{Type: MsgHeartBeatResp, From: r.id, To: m.From, Term: r.Term, Reject: true, Reason: "Term expired"})
		} else {
			r.becomeFollower(m.Term, m.From)
			r.sendMessage(Message{Type: MsgHeartBeatResp, From: r.id, To: m.From, Term: r.Term})
		}
	}
	return nil
}
func (r *raft) stepLeader(m Message) error {
	switch m.Type {
	case MsgHeartBeatResp:
		//TODO
	case MsgAppendResp:
		//TODO
	}
	return nil
}

//Doing
//计时是否超过选举超时时间
func (r *raft) tickElection() {
	r.electionElapsed = r.electionElapsed + time.Second //计时
	//长时间未收到RPC消息
	var m Message
	go func(m Message) {
		m = <-r.MsgChan
	}(m)
	//TODO，收到消息并进行操作
	err := r.step(m)
	//超时，转换为Candidate并开始参加选举
	if err != nil && r.pastElectionTimeout() {
		r.electionElapsed = 0
		log.Println("Election timeout!There is no message received")
		r.becomeCandidate()
	}
}

func (r *raft) tickHeartBeat() {

}

//return true if election timeout
func (r *raft) pastElectionTimeout() bool {
	return r.electionElapsed >= r.randomElectionTimeout
}

//TODO
//RPC回应
func (r *raft) sendMessage(m Message) {
	r.MsgChan <- m
}
