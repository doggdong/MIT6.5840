package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"6.5840/labgob"
	"6.5840/labrpc"
	"log"
	"fmt"
	"os"
	"strconv"
)

// raft state
const(
	Follower = iota
	Candidate
	Leader
	// FollowerReset
)
type logTopic string
const (
	dClient  logTopic = "CLNT"
	dCommit  logTopic = "CMIT"
	dDrop    logTopic = "DROP"
	dError   logTopic = "ERRO"
	dInfo    logTopic = "INFO"
	dLeader  logTopic = "LEAD"
	dLog     logTopic = "LOG1"
	dLog2    logTopic = "LOG2"
	dPersist logTopic = "PERS"
	dSnap    logTopic = "SNAP"
	dTerm    logTopic = "TERM"
	dTest    logTopic = "TEST"
	dTimer   logTopic = "TIMR"
	dTrace   logTopic = "TRCE"
	dVote    logTopic = "VOTE"
	dWarn    logTopic = "WARN"
)
var debugStart time.Time
var debugVerbosity int
func getVerbosity() int {
	v := os.Getenv("VERBOSE")
	level := 0
	if v != "" {
		var err error
		level, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("Invalid verbosity %v", v)
		}
	}
	return level
}
func init() {
	debugVerbosity = getVerbosity()
	debugStart = time.Now()

	log.SetFlags(log.Flags() &^ (log.Ldate | log.Ltime))
}

func DebugLog(topic logTopic, format string, a ...interface{}) {
	if topic != "--TEST"{
	// if topic == "LOG1"{
		return
	}
	time := time.Since(debugStart).Microseconds()
	time /= 100
	prefix := fmt.Sprintf("%06d %v ", time, string(topic))
	format = prefix + format
	log.Printf(format, a...)
}

// as each Raft peer becomes aware that successive log Entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
	CommandTerm  int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// (2A)
	state            int
	term             int
	currentTerm      int
	votedFor         int
	votesReceived    int
	timeReset        int64
	stateReset       bool
	maxTerm          int
	// (2B)
	applyCh          chan ApplyMsg
	log              []Entry
	commitIndex      int
	lastApplied      int
	lastApply		 int
	nextIndex        []int
	MatchIndex       []int
	isCommuicate     []bool
	needReply        [] chan bool
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// (2D)
	lastIncludeIndex      int
	lastIncludeTerm       int
	snapshot               []byte
}

type Entry struct{
	Cmd     interface{}
	Index   int
	Term    int
}

type AppendEntries struct {
	Id                  int
	Term                int

	PreLogIndex         int
	PreLogTerm          int
	Entries             []Entry
	LeaderCommit        int
	
}

type AppendEntriesReply struct {
	State                int
	Term                 int
	Success              bool
	MatchIndex           int
	IsExist             bool
	XTerm                int
	XIndex               int
	Xlen				 int
}

type InstallSnapshotArgs struct {
	Term                 int
	LeaderId             int
	LastIncludeIndex     int
	LastIncludeTerm      int
	Snapshot             []byte
}

type InstallSnapshotReply struct {
	Term                  int
	// NextIndex             int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	rf.mu.Lock()
	defer rf.mu.Unlock()
	// Your code here (2A).
	term = rf.term
	// isleader = (rf.state == Leader)
	if rf.state == Leader{
		isleader = true
	} else {
		isleader = false
	}
	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.term)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	e.Encode(rf.lastIncludeIndex)
	e.Encode(rf.lastIncludeTerm)
	raftstate := w.Bytes()
	rf.persister.Save(raftstate, rf.snapshot)
}


// restore previously persisted state.
func (rf *Raft) readPersist(data []byte, snapshot []byte) {
	// rf.mu.Lock()
	// defer rf.mu.Lock()
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var term int
	var votedFor int
	var log []Entry
	var lastIncludeIndex int
	var lastIncludeTerm int

	if d.Decode(&term) != nil || d.Decode(&votedFor) != nil || d.Decode(&log) != nil ||
		d.Decode(&lastIncludeIndex) != nil || d.Decode(&lastIncludeTerm) != nil{
		DebugLog(dError, "Decode error")
	} else {
		rf.term = term
		rf.votedFor = votedFor
		rf.log = log
		rf.lastIncludeIndex = lastIncludeIndex
		rf.lastIncludeTerm = lastIncludeTerm
		DebugLog(dVote, "S%d [%d] decode from data rf.lastIncludeIndex = %d", rf.me, rf.term, rf.lastIncludeIndex)

	}

	// nil snapshot
	rf.snapshot = snapshot
	// if rf.snapshot!=nil && len(rf.snapshot) > 0 {
	// 	go func(){
	// 		rf.applyCh <- ApplyMsg {
	// 			CommandValid : false,
	// 			SnapshotValid : true,
	// 			Snapshot : rf.snapshot,
	// 			SnapshotTerm : rf.lastIncludeTerm,
	// 			SnapshotIndex : rf.lastIncludeIndex,
	// 		}
	// 	}()
	// }

	rf.lastApply = lastIncludeIndex
	DebugLog(dTimer, "S%d [%d] rf.lastApply = %d", rf.me, rf.term, lastIncludeIndex)

	rf.lastApplied = rf.log[len(rf.log)-1].Index
}



// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

	rf.mu.Lock()
	DebugLog(dVote, "S%d [%d] before snap shot logLen = %d LOG: %v", rf.me, rf.term, len(rf.log), rf.getLogInfo(rf.log))
	cutIndex := index - rf.lastIncludeIndex
	
	// delete log
	rf.lastIncludeIndex = index
	rf.lastIncludeTerm  = rf.log[cutIndex].Term
	rf.log = append(rf.log[:1], rf.log[cutIndex+1:]...)
	rf.log[0].Index = rf.lastIncludeIndex
	rf.log[0].Term = rf.lastIncludeTerm
	DebugLog(dVote, "S%d [%d] after  snap shot logLen = %d LOG: %v", rf.me, rf.term, len(rf.log), rf.getLogInfo(rf.log))

	rf.snapshot = snapshot 

	rf.persist()
	rf.mu.Unlock()

}


// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Id    int
	Term  int
	Index int
	LastLogIndex int
	LastLogTerm int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	GetVote bool
	// VoteFor int
	Term    int
}




// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	DebugLog(dVote, "S%d [%d] recv RequestVote from %d", rf.me, rf.term, args.Id)

	if rf.state == Follower {
		if args.Term > rf.term {
			rf.state = Follower
			rf.term = args.Term
			reply.Term = rf.term
			if rf.votedFor == -1 || rf.CompareTo(args.LastLogIndex, args.LastLogTerm){
				rf.votedFor = args.Id
				rf.stateReset = true
	
				reply.GetVote = true
				DebugLog(dVote, "S%d [%d] Follower (index:%d term:%d) vote --> S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)

			} else  {
				// 不投票
				reply.GetVote = false
				rf.stateReset = false
				DebugLog(dVote, "S%d [%d] Follower (index:%d term:%d) not vote to S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)
			}
		} else if args.Term == rf.term {
			rf.state = Follower
			reply.Term = rf.term
			if rf.votedFor == -1 {
				rf.votedFor = args.Id
				rf.stateReset = true
	
				reply.GetVote = true
				DebugLog(dVote, "S%d [%d] Follower (index:%d term:%d) vote --> S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)

			} else  {
				// 不投票
				reply.GetVote = false
				rf.stateReset = false

				DebugLog(dVote, "S%d [%d] Follower (index:%d term:%d) not vote to S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)
			}
		} else {
			// args.Term < rf.term
			// 不投票, 继续做Follower
	
			reply.Term = rf.term
			reply.GetVote = false
			rf.stateReset = false
			DebugLog(dVote, "S%d [%d] Follower (index:%d term:%d) not vote to S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)
		}
	} else if rf.state == Candidate {
		if args.Term > rf.term {
			rf.state = Follower
			rf.term = args.Term
			reply.Term = rf.term
			if rf.CompareTo(args.LastLogIndex, args.LastLogTerm){
				
				rf.votedFor = args.Id
				reply.GetVote = true
				DebugLog(dVote, "S%d [%d] Candidate (index:%d term:%d) vote --> S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)

			} else  {
				// 不投票
				reply.GetVote = false
				DebugLog(dVote, "S%d [%d] Candidate (index:%d term:%d) not vote to S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)

			}
		}  else  {
			reply.Term = rf.term
			reply.GetVote = false
			DebugLog(dVote, "S%d [%d] Candidate (index:%d) not vote to S%d [%d] (index:%d)", rf.me, rf.term, rf.lastApplied, args.Id, args.Term, args.LastLogIndex)

		}
		// if args.Term > rf.term {

		// 	// 投票
		// 	rf.state = Follower
		// 	rf.term = args.Term
		// 	rf.votedFor = args.Id

		// 	reply.Term = rf.term
		// 	reply.GetVote = true
		// 	DebugLog(dVote, "S%d [%d] Candidate (index:%d) vote --> S%d [%d] (index:%d)", rf.me, rf.term, rf.lastApplied, args.Id, args.Term, args.Index)

		// }  else  {
		// 	reply.Term = rf.term
		// 	reply.GetVote = false
		// 	DebugLog(dVote, "S%d [%d] Candidate (index:%d) not vote to S%d [%d] (index:%d)", rf.me, rf.term, rf.lastApplied, args.Id, args.Term, args.Index)

		// }
	} else if rf.state == Leader {
		if args.Term > rf.term {
			rf.state = Follower
			rf.term = args.Term
			reply.Term = rf.term
			if rf.CompareTo(args.LastLogIndex, args.LastLogTerm){
				rf.votedFor = args.Id
				reply.GetVote = true
				DebugLog(dVote, "S%d [%d] Leader (index:%d term:%d) vote --> S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)

			} else {
				reply.GetVote = false
				DebugLog(dVote, "S%d [%d] Leader (index:%d term:%d) not vote to S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)
			}
		} else {
			reply.Term = rf.term
			reply.GetVote = false
			DebugLog(dVote, "S%d [%d] Leader (index:%d term:%d) not vote to S%d [%d] (index:%d term:%d)", rf.me, rf.term, rf.lastApplied, rf.log[rf.lastApplied - rf.lastIncludeIndex].Term, args.Id, args.Term, args.LastLogIndex, args.LastLogTerm)
		}
		// if args.Term < rf.term {
		// 	// 不投票, 继续做Leader
		// 	reply.Term = rf.term
		// 	reply.GetVote = false
		// 	DebugLog(dVote, "S%d [%d] Leader (index:%d) not vote to S%d [%d] (index:%d)", rf.me, rf.term, rf.lastApplied, args.Id, args.Term, args.Index)

		// } else if rf.CompareTo(args.LastLogIndex, args.LastLogTerm){
		// 	rf.state = Follower
		// 	rf.votedFor = args.Id
		// 	rf.term = args.Term

		// 	reply.Term = rf.term
		// 	reply.GetVote = true
		// 	DebugLog(dVote, "S%d [%d] Leader vote --> %d", rf.me, rf.term, args.Id)

		// } else  {
		// 	reply.Term = rf.term
		// 	reply.GetVote = false
		// 	DebugLog(dVote, "S%d [%d] Candidate (index:%d) not vote to S%d [%d] (index:%d)", rf.me, rf.term, rf.lastApplied, args.Id, args.Term, args.Index)

		// }
	}
	rf.persist()
}

func (rf *Raft) CompareTo(lastLogIndex int, lastLogTerm int) bool {
	currLogTerm := rf.log[rf.lastApplied - rf.lastIncludeIndex].Term
	currLogIndex := rf.lastApplied
	
	// compare Term
	if currLogTerm < lastLogTerm {
		// candiate is newer than current
		DebugLog(dVote, "S%d [%d] Compare vote, lastLogTerm is lower than candidate", rf.me, rf.term)
		return true
	} else if currLogTerm > lastLogTerm {
		DebugLog(dVote, "S%d [%d] Compare not vote, lastLogTerm is highter than candidate", rf.me, rf.term)

		return false
	} else {
		// currLogTerm == lastLogTerm
		if currLogIndex <= lastLogIndex {
			DebugLog(dVote, "S%d [%d] Compare (index:%d) vote, index is lower than candidate", rf.me, rf.term, rf.lastApplied)
			return true
		} else {
			DebugLog(dVote, "S%d [%d] Compare (index:%d) not vote, index is highter than candidate", rf.me, rf.term, rf.lastApplied)

			return false
		} 
	}


}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	// do not acquire lock
	// DebugLog(dTimer, "S%d [%d] call rpc of S%d", rf.me, rf.term, server)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) AppendEntries(args *AppendEntries, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state == Leader {
		DebugLog(dLog, "S%d [%d] Leader, LOG: %v", rf.me, rf.term, rf.getLogInfo(rf.log))
	} else if rf.state == Candidate {
		DebugLog(dLog, "S%d [%d] Candidate, LOG: %v", rf.me, rf.term, rf.getLogInfo(rf.log))
	} else {
		DebugLog(dLog, "S%d [%d] Follower, LOG: %v", rf.me, rf.term, rf.getLogInfo(rf.log))
	}

	if rf.state == Leader {
		if args.Term > rf.term {
			// if rf.CompareTo()
			reply.State = rf.state
			rf.state = Follower
			rf.term = args.Term
		} else {
			reply.Success = false
			reply.Term = rf.term
			reply.State = rf.state
			DebugLog(dError, "S%d [%d] Leader, recv lower heartBeat from S%d", rf.me, rf.term, args.Id)
		}
	} else if rf.state == Candidate {
		if args.Term >= rf.term {
			reply.State = rf.state
			rf.state = Follower
			rf.term = args.Term
		} else {
			reply.Success = false
			reply.Term = rf.term
			reply.State = rf.state
			DebugLog(dError, "S%d [%d] Cadidate, recv lower heartBeat", rf.me, rf.term)
		}
	} else if rf.state == Follower {
		if args.Term >= rf.term {
			rf.state = Follower
			rf.stateReset = true
			rf.term = args.Term
			rf.followerHandleAE(args, reply)
		} else {
			// 收到低级别的心跳
			DebugLog(dError, "S%d [%d] Follower, recv lower heartBeat", rf.me, rf.term)
			reply.Success = false
			reply.Term = rf.term
			reply.State = rf.state
		}
	}
	rf.persist()

}

// 由于,只有nextIndex比lastIncludeIndex大,才会发送AE,执行到此处
// 所以收到的args中的preIndex最小为 leader的lastIncludeIndex

// nextIndex 不会比follower的lastIncludeIndex小, 因为 lastIncludeIndex < commitIndex, 所以不会发生
// 则preIndex 最小只会是 commitIndex >= lastIncludeIndex

func (rf *Raft) followerHandleAE(args *AppendEntries, reply *AppendEntriesReply) {
	// DebugLog(dError, "S%d [%d] hendle AE", rf.me, rf.term)
	reply.Term = rf.term
	reply.State = rf.state

	if rf.lastApplied >= args.PreLogIndex{
		// DebugLog(dLog, "S%d [%d] Follower, need to delete some log", rf.me, rf.term)
		if rf.commitIndex > args.PreLogIndex{
			
			DebugLog(dLog, "S%d [%d] Follower, recv index error rf.commitIndex = %d, PreLogIndex = %d", rf.me, rf.term, rf.commitIndex, args.PreLogIndex)
		}
		if rf.log[args.PreLogIndex - rf.lastIncludeIndex].Term == args.PreLogTerm {
			// pre is match
			// // 1. delete log 
			// DebugLog(dLog, "S%d [%d] Follower, return true, pre match, Index = %d ", rf.me, rf.term, args.PreLogIndex)

			// rf.log = rf.log[:args.PreLogIndex+1 - rf.lastIncludeIndex]
			// rf.lastApplied = args.PreLogIndex

			// // 2. append log
			// for i:=0; i<len(args.Entries); i++ {
			// 	if(args.Entries[i].Cmd != nil) {
			// 		rf.log = append(rf.log, args.Entries[i])
			// 		rf.lastApplied++
			// 		DebugLog(dTest, "S%d [%d] Follower recv AE form S%d add log, index: %d ", rf.me, rf.term, args.Id, args.Entries[i].Index)
			// 	} else {
			// 		DebugLog(dLog, "S%d [%d] Follower recv empty AE form S%d", rf.me, rf.term, args.Id)
			// 	}
			// }
			// 先删除log,再append log, 改为原地修改log,长度不够再append log
			DebugLog(dLog, "S%d [%d] Follower, return true, pre match, Index = %d ", rf.me, rf.term, args.PreLogIndex)
			matchIndex := args.PreLogIndex
			for i:=0; i<len(args.Entries); i++ {
				if(args.Entries[i].Cmd != nil) {
					if args.PreLogIndex + i + 1 <= rf.lastApplied {
						// 原地覆盖
						rf.log[args.PreLogIndex + i + 1 - rf.lastIncludeIndex] = args.Entries[i]
					} else {
						rf.log = append(rf.log, args.Entries[i])
						rf.lastApplied++
					}
					matchIndex++
					DebugLog(dTest, "S%d [%d] Follower recv AE form S%d add a log, index: %d ", rf.me, rf.term, args.Id, args.Entries[i].Index)
				} else {
					DebugLog(dLog, "S%d [%d] Follower recv empty AE form S%d", rf.me, rf.term, args.Id)
				}
			}
			DebugLog(dLog, "S%d [%d] Follower recv AE form S%d add log len = %d, index: %d", rf.me, rf.term, args.Id, len(args.Entries), args.Entries[len(args.Entries)-1].Index)

			// 3. update commitIndex
			if(args.LeaderCommit > rf.commitIndex){
				DebugLog(dLog, "S%d [%d] Follower need to apply LOG, leaderCommit= %d, selfCommit= %d", rf.me, rf.term, args.LeaderCommit, rf.commitIndex)

				if(rf.lastApplied < args.LeaderCommit){
					rf.commitIndex = rf.lastApplied
				} else {
					rf.commitIndex = args.LeaderCommit
				}
				
				start := rf.lastApply+1
				end := rf.commitIndex
				logCopy := make([]Entry, end - start + 1)
				copy(logCopy, rf.log[start:end+1])
				DebugLog(dTest, "S%d [%d] Follower before apply LOG rf.lastApply= %d", rf.me, rf.term, rf.lastApply)

				rf.lastApply = end
				rf.mu.Unlock()
				for logId:= start; logId <= end; logId++ {
					msg := ApplyMsg{
						CommandValid : true,
						Command : logCopy[logId - start - rf.lastIncludeIndex].Cmd,
						CommandIndex : logId,
						CommandTerm : logCopy[logId - start - rf.lastIncludeIndex].Term,
					}
					// rf.lastApply++

					// DebugLog(dTest, "S%d [%d] Follower apply LOG, CommandIndex= %d rf.lastApply= %d", rf.me, rf.term, logId, rf.lastApply)
					// fmt.Println(dTest, "S%d [%d] Follower apply LOG, CommandIndex= %d rf.lastApply= %d", rf.me, rf.term, logId, rf.lastApply)
					rf.applyCh <- msg
				}

				rf.mu.Lock()

			}

			reply.Success = true
			// reply.MatchIndex = rf.lastApplied
			reply.MatchIndex = matchIndex
			DebugLog(dTest, "S%d [%d] Follower recv AE args.PreLogIndex = %d return MatchIndex= %d", rf.me, rf.term, args.PreLogIndex, reply.MatchIndex)

			
		} else {
			// pre not match
			// delete [args.PreLogIndex, end]
			DebugLog(dLog, "S%d [%d] Follower, preIndex: %d not match rf.lastApplied = %d", rf.me, rf.term, args.PreLogIndex, rf.lastApplied)
			
			// get XTerm XIndex
			reply.XTerm = rf.log[args.PreLogIndex - rf.lastIncludeIndex].Term
			reply.XIndex = args.PreLogIndex
			for rf.log[reply.XIndex - rf.lastIncludeIndex].Term == reply.XTerm {
				reply.XIndex--
				// if reply.XIndex - rf.lastIncludeIndex == -1{
				// 	break
				// }
			}
			reply.XIndex++
			DebugLog(dLog, "S%d [%d] Follower, return false, preIndex =  %d  not match, XTerm = %d, XIndex = %d", rf.me, rf.term, args.PreLogIndex, reply.XTerm, reply.XIndex)

			rf.log = rf.log[:args.PreLogIndex - rf.lastIncludeIndex]
			rf.lastApplied = args.PreLogIndex-1
			reply.Success = false
			reply.MatchIndex = rf.lastApplied
		}
		
	} else {
		// rf.lastApplied < args.PreLogIndex
		// preIndex not exist
		DebugLog(dLog, "S%d [%d] Follower, return false, rf.lastApplied = %d < PreLogIndex = %d", rf.me, rf.term, rf.lastApplied, args.PreLogIndex)
		
		reply.XTerm = -1
		reply.XIndex = -1
		reply.Success = false
		reply.MatchIndex = rf.lastApplied
	}
	DebugLog(dLog, "S%d [%d] Follower, lastApplied = %d, LOG: %v", rf.me, rf.term, rf.lastApplied, rf.getLogInfo(rf.log))

}

func (rf *Raft) getLogInfo(log []Entry) string{
	s :="[ "
	for _, l := range log{
		s +=  fmt.Sprint("[",l.Index," ", l.Term,"]","{", l.Cmd,"}")
		s += ", "
	}
	s += "]"
	return s
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntries, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	// DebugLog(dLog, "S%d [%d] get respose from S%d", rf.me, rf.term, server)

	return ok
}



// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	// index := -1
	// term := -1
	// isLeader := true
	var isLeader bool
	var index int
	var term  int
	// Your code here (2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state == Leader {
		isLeader = true
		newEntry := Entry{
			Cmd   : command,
			Index : rf.lastApplied+1,
			Term  : rf.term,
		}
		rf.log = append(rf.log, newEntry)
		rf.lastApplied++
		index = rf.lastApplied
		term  = rf.term
		rf.persist()
		// DebugLog(dLog, "S%d [%d] get cmd LOG++: %v, index = %d LOG : %v", rf.me, rf.term, command, rf.lastApplied, rf.getLogInfo(rf.log))
		DebugLog(dTest, "S%d [%d] get cmd LOG++: Raftindex = %d", rf.me, rf.term, rf.lastApplied,)
		
		currPeersNum := len(rf.peers)
		currId := rf.me
		rf.mu.Unlock()
		for id := 0; id < currPeersNum; id++ {
			if(id == currId){
				continue
			}
			// DebugLog(dTest, "S%d [%d] start to send AE for S%d", rf.me, rf.term, id)
			rf.needReply[id] <- true
		}
		rf.mu.Lock()

		// for id := 0; id < currPeersNum; id++ {
		// 	if(id == currId){
		// 		continue
		// 	}
		// 	// rf.nextIndex[id] = rf.lastApplied + 1
		// 	// DebugLog(dTest, "S%d [%d] start to send AE for S%d", rf.me, rf.term, id)
		// 	go rf.SendOneAE(id)
		// }

		// for id := 0; id < len(rf.peers); id++ {
		// 	if(id == rf.me){
		// 		continue
		// 	}
		// 	DebugLog(dTest, "S%d [%d] start to send AE for S%d", rf.me, rf.term, id)
		// 	// 这里的解锁操作会导致前后的rf.lastApplied不一致
		// 	// 所以在解锁之前,提前将index, term保存
		// 	rf.mu.Unlock()
		// 	rf.needReply[id] <- true
		// 	rf.mu.Lock()

		// }
	} else {
		isLeader = false
		index = rf.lastApplied
		term  = rf.term
	}


	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) startElection() {
	// 1. send RequestVote to peers
	rf.mu.Lock()
	rf.votesReceived = 1
	currPeersNum := len(rf.peers)
	candidateId := rf.me
	rf.mu.Unlock()

	for id := 0; id < currPeersNum; id++ {
		if(id == candidateId){
			continue
		}
		DebugLog(dTimer, "S%d candidate , send VoteRequest to peers %d", candidateId, id)
		go func(id int){
			rf.mu.Lock()
			lastLogIndex := 0
			lastLogTerm  := 0
			if rf.lastApplied - rf.lastIncludeIndex == 0{
				// has been snapshot
				lastLogIndex = rf.lastIncludeIndex
				lastLogTerm = rf.lastIncludeTerm
			} else {
				lastLogIndex = rf.lastApplied
				lastLogTerm = rf.log[rf.lastApplied - rf.lastIncludeIndex].Term
			}
			args := RequestVoteArgs{
				Id: rf.me,
				Term: rf.term,
				Index: rf.lastApplied,
				LastLogIndex: lastLogIndex,
				LastLogTerm: lastLogTerm,
			}
			reply := RequestVoteReply{
				GetVote : false,
				Term: -1,
			}
			rf.mu.Unlock()
			
			ok := rf.sendRequestVote(id, &args, &reply)

			rf.mu.Lock()
			if ok {

				DebugLog(dTimer, "S%d [%d] candidate, get reply from: %d", rf.me, rf.term, id)

				if reply.GetVote {

					rf.votesReceived++
					DebugLog(dTimer, "S%d [%d] candidate, get vote: %d", rf.me, rf.term, rf.votesReceived)
				}
			} else {		
				// DebugLog(dTimer, "S%d [%d] send Request Vote to S%d error", rf.me, rf.term, id)
			}
			rf.mu.Unlock()
			return 
			
		}(id)
		
	}
	rf.mu.Lock()
	DebugLog(dTimer, "S%d [%d] candidate, election over", rf.me, rf.term)
	rf.mu.Unlock()

}


func (rf *Raft) LeaderSendAppendEntries(leaderId int, currPeersNum int) {
	// var wg sync.WaitGroup
	for id := 0; id < currPeersNum; id++ {
		if(id == leaderId){
			continue
		}
		
		go rf.SendAEToPeer(id)
	}
}

func (rf *Raft) SendAEToPeer(id int) {
	ticker := time.NewTicker(1 * time.Millisecond)
	for {

		select {
		case <-rf.needReply[id]:
		// 	// peer reply, need send log quickly
			DebugLog(dTest, "S%d [%d] Leader, Resent to S%d, because message ", rf.me, rf.term, id)

		case <-ticker.C:
			// DebugLog(dTimer, "S%d [%d] Leader, retry send to S%d", rf.me, rf.term, id)
			DebugLog(dTest, "S%d [%d] Leader, ReSend to S%d, because timeout ", rf.me, rf.term, id)
			for len(rf.needReply[id]) == 1 {
				<-rf.needReply[id]
			}
		}

		ticker.Stop()
		ticker.Reset(200*time.Millisecond)

		rf.mu.Lock()
		if rf.state != Leader || rf.killed() {
			DebugLog(dTimer, "S%d [%d] not leader anymore", rf.me, rf.term)

			rf.mu.Unlock()
			return
		}
		nextIndex := rf.nextIndex[id]
		snapLastIndex := rf.lastIncludeIndex
		rf.mu.Unlock()

		if nextIndex - snapLastIndex <= 0{
			// nextIndex has been snapshot
			// send installSnapshot
			go rf.SendInstallSnapshot(id)
		} else {
			// send args
			go rf.SendOneAE(id)
		}
		
	}			
	// DebugLog(dTimer, "S%d Leader send AE to S%d over", rf.me, id)
}

func (rf *Raft) SendInstallSnapshot(id int) bool{
	rf.mu.Lock()
	args := InstallSnapshotArgs{
		Term : rf.term,
		LeaderId : rf.me,
		LastIncludeIndex : rf.lastIncludeIndex,
		LastIncludeTerm : rf.lastIncludeTerm,
		Snapshot : rf.snapshot,
	}
	reply := InstallSnapshotReply{}
	DebugLog(dTimer, "S%d Leader send SnapShot to S%d ", rf.me, id)

	rf.mu.Unlock()

	ok := rf.peers[id].Call("Raft.InstallSnapshot", &args, &reply)

	rf.mu.Lock()
	if ok {
		rf.nextIndex[id] = rf.lastIncludeIndex + 1
	}
	rf.mu.Unlock()
	return ok
}

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply){
	// 0. check snapshot is old
	rf.mu.Lock()
	DebugLog(dTimer, "S%d  Install SnapShot args.LastIncludeIndex = %d, LOG: %v", rf.me, args.LastIncludeIndex, rf.getLogInfo(rf.log))

	reply.Term = rf.term
	if args.Term < rf.term {
		rf.mu.Unlock()
		return
	}
	if args.LastIncludeIndex <= rf.lastIncludeIndex {
		// this snap has been in server
		rf.mu.Unlock()
		return
	}

	// args snap is longer than current snap
	rf.lastIncludeIndex = args.LastIncludeIndex
	rf.lastIncludeTerm = args.LastIncludeTerm
	if rf.lastApplied <= args.LastIncludeIndex {
		// args snap is longer than current logIndex
		rf.log[0].Index = args.LastIncludeIndex
		rf.log[0].Term = args.LastIncludeTerm
		// delete all of log
		rf.log = rf.log[:1]
		rf.lastApplied = args.LastIncludeIndex
		DebugLog(dTimer, "S%d  log is behind ", rf.me)
		rf.commitIndex =  args.LastIncludeIndex

	} else {
		// shorter than current logIndex
		// delete part of log
		DebugLog(dTimer, "S%d  log need to cut ", rf.me)
		rf.log = append(rf.log[:1], rf.log[args.LastIncludeIndex+1:]...)
		if rf.commitIndex < args.LastIncludeIndex {
			rf.commitIndex =  args.LastIncludeIndex
		}
	}

	rf.persist()

	rf.mu.Unlock()
	DebugLog(dTimer, "S%d  apply snapshot start ", rf.me)

	// 1. decode snapshot
	// 2. apply log to applyCh
	msg := ApplyMsg {
		CommandValid : false,
		SnapshotValid : true,
		Snapshot : args.Snapshot,
		SnapshotTerm : args.LastIncludeTerm,
		SnapshotIndex : args.LastIncludeIndex,
	}
	rf.applyCh <- msg
	
	rf.mu.Lock()
	rf.lastApply = args.LastIncludeIndex
	DebugLog(dTimer, "S%d  apply snapshot over , lastIndex = rf.lastApply = %d", rf.me, args.LastIncludeIndex)
	rf.mu.Unlock()

}


func (rf *Raft) SendOneAE(id int) {
	rf.mu.Lock()
	// DebugLog(dTest, "S%d [%d] Leader, send AE to S%d, args.PreLogIndex = %d lastIncludeIndex = %d", rf.me, rf.term, id, rf.nextIndex[id]-1, rf.lastIncludeIndex)
	// DebugLog(dTest, "S%d [%d] Leader, send AE to S%d, args.PreLogIndex = %d lastIncludeIndex = %d", rf.me, rf.term, id, rf.nextIndex[id]-1, rf.lastIncludeIndex)

	args := AppendEntries{
		Id: rf.me,
		Term: rf.term,
		LeaderCommit : rf.commitIndex,
	}
	// 1. set preIndex preTerm 
	// 2. set entry
	// rf.nextIndex[id] = rf.lastApplied+1
	// args.PreLogIndex = rf.lastApplied;

	if rf.nextIndex[id] - rf.lastIncludeIndex <= 0{
		rf.mu.Unlock()
		return
	}
	args.PreLogIndex = rf.nextIndex[id]-1;
	args.PreLogTerm = rf.log[args.PreLogIndex - rf.lastIncludeIndex].Term

	logIndex := rf.nextIndex[id]

	if logIndex > rf.lastApplied+1 {
		DebugLog(dError, "S%d [%d] Leader, logIndex ERROR", rf.me, rf.term)
		return
	} else if logIndex == rf.lastApplied+1 {
		// first heartBeat or has matched over
		DebugLog(dTest, "S%d [%d] Leader, send heartBeat to %d, length = %d args.PreLogIndex = %d", rf.me, rf.term, id, logIndex-args.PreLogIndex - 1, args.PreLogIndex)

		emptyEntry := Entry{
			Cmd   : nil,
			Index : logIndex,
			Term  : rf.term,
		}
		args.Entries = append(args.Entries, emptyEntry)
	} else {
		// logIndex < rf.lastApplied+1
		for logIndex < rf.lastApplied + 1{
			args.Entries = append(args.Entries, rf.log[logIndex - rf.lastIncludeIndex])
			logIndex++
		}
		DebugLog(dTest, "S%d [%d] Leader, send AE to S%d, args.PreLogIndex = %d args.Entries size = %d", rf.me, rf.term, id, args.PreLogIndex, len(args.Entries))
	}

	reply := AppendEntriesReply{}
	// DebugLog(dTimer, "S%d [%d] Leader, send heartBeat to peers %d, args.PreLogIndex = %d LOG: %v", rf.me, rf.term, id, args.PreLogIndex,rf.getLogInfo(rf.log))
	rf.mu.Unlock()

	// rpc调用期间不持有锁
	ok := rf.sendAppendEntries(id, &args, &reply)
	
	rf.mu.Lock()
	if ok {
		if reply.Term > rf.term{
			// 发现有更高任期的节点
			DebugLog(dTimer, "S%d [%d] Leader find higher Term %d", rf.me, rf.term, reply.Term)

			if reply.Term > rf.maxTerm {
				rf.maxTerm = reply.Term
			}
			rf.mu.Unlock()
			return
		}

		if reply.State != Follower {
			rf.mu.Unlock()
			return
		}

		if reply.Success {
			if rf.MatchIndex[id] < reply.MatchIndex{
				rf.MatchIndex[id] = reply.MatchIndex
			}
			rf.nextIndex[id] = reply.MatchIndex + 1
			// if rf.nextIndex[id] < reply.MatchIndex + 1{
			// }
			DebugLog(dTest, "S%d [%d] Leader commitIndex: %d recv matchIndex: %d ", rf.me, rf.term, rf.commitIndex, reply.MatchIndex)

			// check majority, try to commit leader log
			if rf.commitIndex < rf.MatchIndex[id] {
				localMatchId := rf.MatchIndex[id]
				// 只能提交当前任期, 旧任期log需要随着当前任期一起提交
				// if rf.log[localMatchId].Term == rf.term || args.Entries[0].Cmd == nil{
				// 可以改成 比较args.Entries最后一位 比较
				if rf.log[localMatchId - rf.lastIncludeIndex].Term == rf.term {
					DebugLog(dTest, "S%d [%d] Leader is ready to commit index = %d ", rf.me, rf.term, localMatchId)

					if(args.Entries[0].Cmd == nil) {
						DebugLog(dLog, "S%d [%d] Leader send empty AE over, now commit: %d ", rf.me, rf.term, localMatchId)
					} else {
						DebugLog(dLog, "S%d [%d] Leader send  log  AE over, now commit: %d ", rf.me, rf.term, localMatchId)
					}

					// last log belone to curr term
					// normal heartBeat
					nCount := 0
					// 检查刚刚确认匹配的log[rf.MatchIndex[id]] 是否已经匹配到大多数节点
					for peerId, matched := range rf.MatchIndex {
						if peerId == rf.me {
							continue
						}
						if(matched >= localMatchId){
							nCount++
						} 
					}
					
					if(nCount >= len(rf.peers)/2) {
						// leader need to commit
						rf.commitIndex = localMatchId
						DebugLog(dLog, "S%d Leader [%d] apply log , Index -> %d", rf.me, rf.term, rf.commitIndex)
					}
				}

			} 

			DebugLog(dTest, "S%d [%d] Leader get AE reply, nextIndex[%d] %d ", rf.me, rf.term, id, rf.nextIndex[id])
			
			if(rf.nextIndex[id] <= rf.lastApplied){
				// not match over
				// rf.needReply[id]<-true
				// if len(rf.needReply[id]) != 1{
				// 	rf.needReply[id]<-true
				// }
			}
		} else {
			// reply.Success = false
			// 1. hight term follower  -->  has already return
			// 2. preLog not match     -->  continue to send AE
			// 3. preLog not exist     -->  continue to send AE
			if reply.XTerm != -1 {
				// preLog not match
				// 1. check if leader has log with XTerm
				// 2. if leader has XTerm, find the last XTerm index, nextIndex[id] = last XTerm index + 1
				// 3. if leader not has XTerm, nextIndex[id] = XIndex
				index := rf.lastApplied - rf.lastIncludeIndex
				for index > 0 {
					if(rf.log[index].Term == reply.XTerm){
						break
					}
					index--
				}
				if index != 0{
					// leader find XTerm
					DebugLog(dTimer, "S%d [%d] Leader find XTerm(%d), nextIndex = %d for S%d ", rf.me, rf.term, reply.XTerm, index+1, id)

					rf.nextIndex[id] = index + 1
				} else {
					// leader not have XTerm
					// reply.XIndex is fist index with XTerm in follower's log
					DebugLog(dTimer, "S%d [%d] Leader not find XTerm(%d), nextIndex = %d for S%d ", rf.me, rf.term, reply.XTerm, reply.XIndex, id)

					rf.nextIndex[id] = reply.XIndex
				}
			} else {
				// follower log is too short
				rf.nextIndex[id] = reply.MatchIndex + 1
			}
			

			DebugLog(dLog, "S%d [%d] Leader nextIndex[%d]--: %d ", rf.me, rf.term, id, rf.nextIndex[id])

			// if len(rf.needReply[id]) != 1{
			// 	rf.needReply[id]<-true
			// }
			
			if (rf.nextIndex[id]<1){
				rf.nextIndex[id] = 1
			}
		}
	} else {
		// 网络原因发送失败
		// DebugLog(dTimer, "S%d Leader send AE to S%d error", rf.me, id)
	}
	rf.mu.Unlock()
}

func (rf *Raft) beFollower(term int) {
	rf.mu.Lock()
	rf.state = Follower
	rf.term = term
	rf.stateReset = false
	// rf.votedFor = -1
	timeDuration := 300 + (rand.Int63() % 300)
	// timeDuration := 150 + (rand.Int63() % 150)
	rf.timeReset = time.Now().UnixMilli()
	rf.persist()

	rf.mu.Unlock()


	ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()
	// Check if a leader election should be started.
	for {
		<-ticker.C
		rf.mu.Lock()
		// DebugLog(dTimer, "S%d [%d] follower, CHECK", rf.me, rf.term)
		
		// 收到心跳 收到投票请求, 重新开始当follower
		if rf.state == Follower && rf.stateReset {
			DebugLog(dTimer, "S%d [%d] follower, restart be follower", rf.me, rf.term)
			go rf.beFollower(rf.term)
			rf.mu.Unlock()
			return
		}
		if rf.state != Follower{
			DebugLog(dTimer, "S%d [%d] follower, not be follower", rf.me, rf.term)
			rf.mu.Unlock()

			return
		}
		if time.Now().UnixMilli() - rf.timeReset > timeDuration{
			// times out, starts election
			// term increase by itself
			DebugLog(dTimer, "S%d [%d] follower, times out , be candidate %d %d %d ", rf.me, rf.term, timeDuration, time.Now().UnixMilli(), rf.timeReset)

			go rf.beCandidate(rf.term+1)
			rf.mu.Unlock()
			return
		}
		if rf.killed() {
			rf.mu.Unlock()
			return
		}
		rf.mu.Unlock()
	}
}

func (rf *Raft) beCandidate(term int) {
	rf.mu.Lock()	
	rf.state = Candidate
	rf.term = term
	rf.votedFor = rf.me
	rf.votesReceived = 0
	rf.stateReset = false
	rf.timeReset = time.Now().UnixMilli()
	rf.persist()

	rf.mu.Unlock()
	
	// timeDuration := 300 + (rand.Int63() % 150)
	timeDuration := 500 + (rand.Int63() % 300)
	ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()


	rf.startElection()
	for {
		<-ticker.C

		rf.mu.Lock()
		
		if rf.state != Candidate {
			DebugLog(dTimer, "S%d [%d] candidate, not candidate anymore", rf.me, rf.term)
			if rf.state == Follower {
				DebugLog(dTimer, "S%d [%d] candidate,  be follow",rf.me, rf.term)
				go rf.beFollower(rf.term)
			} else {
				DebugLog(dError, "S%d [%d] candidate, state change to other", rf.me, rf.term)
			}
			rf.mu.Unlock()
			return
		}

		if rf.votesReceived >= (len(rf.peers)+1)/2 {
			// receives votes 
			DebugLog(dTimer, "S%d [%d] candidate, get enough vote %d",rf.me, rf.term, rf.votesReceived)

			go rf.beLeader()
			// break
			rf.mu.Unlock()
			return
		} 

		if time.Now().UnixMilli() - rf.timeReset > timeDuration {
			// times out, new election
			DebugLog(dTimer, "S%d [%d] candidate, times out continue candidate",rf.me, rf.term)

			go rf.beCandidate(rf.term+1)
			// break
			rf.mu.Unlock()
			return
		} 

		if rf.killed() {
			return
		}
		rf.mu.Unlock()
	}
}

func (rf *Raft) beLeader() {
	
	rf.mu.Lock()
	DebugLog(dTimer, "S%d [%d], became leader !",rf.me, rf.term)

	rf.state = Leader
	rf.votesReceived = 0
	rf.stateReset = false
	currPeersNum := len(rf.peers)
	// currCommitIndex := rf.commitIndex
	leaderId := rf.me
	rf.maxTerm = rf.term

	// rf.lastApplied = len(rf.log)-1
	rf.lastApplied = rf.log[len(rf.log)-1].Index
	for i := 0; i<currPeersNum; i++ {
		if(i==rf.me){
			continue
		} else {
			rf.nextIndex[i] = rf.lastApplied+1
		}
		// rf.isCommuicate[i] = false
	}
	rf.persist()
	rf.mu.Unlock()

	ticker := time.NewTicker(10 * time.Millisecond)
    defer ticker.Stop()

	rf.LeaderSendAppendEntries(leaderId, currPeersNum)
	
	for {
		<-ticker.C
		
		rf.mu.Lock()
		if rf.state != Leader {
			go rf.beFollower(rf.term)
			rf.mu.Unlock()
			return
		}

		if rf.maxTerm > rf.term {
			rf.state = Follower
			rf.term = rf.maxTerm
			DebugLog(dTimer, "S%d Leader to Follower", leaderId)
			go rf.beFollower(rf.term)
			rf.mu.Unlock()
			return 
		}
		if rf.killed() {
			DebugLog(dLog2, "S%d [%d] killed !",rf.me, rf.term)
			rf.mu.Unlock()
			return
		}

		// if currCommitIndex != rf.commitIndex {
		if rf.lastApply != rf.commitIndex {
			start := rf.lastApply + 1
			end := rf.commitIndex
			logCopy := make([]Entry, end - start + 1)
			copy(logCopy, rf.log[start: end+1])
			rf.lastApply = end
			rf.mu.Unlock()

			for logId := start; logId <= end; logId++ {
				// apply message
				DebugLog(dLog, "S%d [%d] Leader apply LOG logId: %d, lastIncludeIndex: %d", rf.me, rf.term, logId, rf.lastIncludeIndex)
				msg := ApplyMsg{
					CommandValid : true,
					Command : logCopy[logId - start - rf.lastIncludeIndex].Cmd,
					CommandIndex : logId,
					CommandTerm : logCopy[logId - start - rf.lastIncludeIndex].Term,
				}

				DebugLog(dTest, "S%d [%d] Leader apply LOG, CommandIndex= %d rf.lastApply = %d", rf.me, rf.term, logId, rf.lastApply)

				rf.applyCh <- msg
			}
			rf.mu.Lock()
		}
		DebugLog(dLog, "S%d [%d] Leader, LOG: %v", rf.me, rf.term, rf.getLogInfo(rf.log))
		DebugLog(dTest, "S%d [%d] Leader, lastApplied=%d, rf.commitIndex=%d", rf.me, rf.term, rf.lastApplied, rf.commitIndex)

		rf.mu.Unlock()
	}
}

func (rf *Raft) ticker(term int) {
	// currentTerm := rf.term
	// if rf.snapshot!=nil && len(rf.snapshot) > 0 {
	// 	DebugLog(dLog, "S%d [%d] start apply snapshot", rf.me, rf.term)

	// 	rf.applyCh <- ApplyMsg {
	// 		CommandValid : false,
	// 		SnapshotValid : true,
	// 		Snapshot : rf.snapshot,
	// 		SnapshotTerm : rf.lastIncludeIndex,
	// 		SnapshotIndex : rf.lastIncludeTerm,
	// 	}
		
	// } else {
	// 	DebugLog(dLog, "S%d [%d] snapshot is empty", rf.me, rf.term)

	// }
	// DebugLog(dLog, "S%d [%d] finish apply snapshot", rf.me, rf.term)
	
	rf.beFollower(term)
}
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {

	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.dead = 0

	// Your initialization code here (2A, 2B, 2C).
	rf.state = Follower
	rf.term = 0
	rf.votedFor = -1
	rf.votesReceived = 0
	rf.stateReset = false
	rf.maxTerm = -1
	// rf.recvRequestVote = false
	// (2B)
	rf.applyCh = applyCh
	rf.nextIndex = make([]int, len(peers))
	rf.MatchIndex = make([]int, len(peers))
	// rf.isCommuicate = make([]bool, len(peers))

	rf.needReply = make([]chan bool, len(peers))    
	for i:=0; i<len(peers); i++ {    
		rf.needReply[i] = make(chan bool, 1)    
	} 
	// rf.needReply = make([]chan bool, len(peers))
	emptyEntry := Entry{
		Cmd   : nil,
		Index : rf.lastApplied,
		Term  : rf.term,
	}
	rf.log = append(rf.log, emptyEntry)
	rf.lastApplied = 0
	rf.lastApply = 0
	rf.commitIndex = 0

	rf.lastIncludeIndex = 0
	rf.lastIncludeTerm = 0
	
	DebugLog(dLog, "S%d [%d] alive ! ", rf.me, rf.term)


	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState(), persister.ReadSnapshot())
	DebugLog(dTimer, "S%d [%d] restart ! lastApplied = %d", rf.me, rf.term, rf.lastApplied)

	// start ticker goroutine to start elections
	go rf.ticker(rf.term)


	return rf
}
