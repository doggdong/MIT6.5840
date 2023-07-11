package kvraft

import (
	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
	"log"
	"sync"
	"sync/atomic"

	"time"
)


const Debug = true
func DPrintf(format string, a ...interface{}) (n int, err error) {
	// log.SetFlags(log.Lmicroseconds)
	if Debug {
		log.Printf(format, a...)
	}
	return
}


type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Cmd          string
	Key          string
	Value        string
	ClientId     int64
	MsgId        int
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	kvDB           map[string]string
	waitCh         map[int]chan Op    //raftIndex -> waitChannle
	LastCmdIndex   map[int64]int      // clientId -> commandIndex
}

func (kv *KVServer) ApplyToChannel() {
	for log := range kv.applyCh {
		if log.CommandValid {
			kv.mu.Lock()
			// check if the wait channel is exist
			// log.commandIndex is same as raftIndex
			DPrintf("server[%d] ApplyToChannel recv, Raftindex = %d, MsgIndex = %d, value: %v", kv.me, log.CommandIndex, (log.Command.(Op)).MsgId, (log.Command.(Op)).Value)

			waitCh, IsExist := kv.waitCh[log.CommandIndex]
			kv.mu.Unlock()
			if !IsExist{
				// waitCh has been delete
				// DPrintf("server[%d] channel wait for, Raftindex = %d, has been delete", kv.me, log.CommandIndex)
			} else {
				// send to waitCh
				waitCh <- log.Command.(Op)
			}
		} else if log.SnapshotValid {

		}

		if kv.killed(){
			kv.mu.Lock()
			DPrintf("server[%d] has killed", kv.me)
			kv.mu.Unlock()
			
			return
		}
	}
}


func (kv *KVServer) Get(args *RpcArgs, reply *RpcReply) {
	// Your code here.
	// append log to raft
	kv.mu.Lock()
	DPrintf("server[%d] Get, ClientId= %d, MsgId= %d", kv.me, args.ClientId, args.MsgId)
	kv.mu.Unlock()

	log := Op{
		Cmd : args.Cmd,
		Key : args.Key,
		Value : "",
		ClientId : args.ClientId,
		MsgId : args.MsgId,
	}
	kv.commitLog(args, reply, &log)
}

func (kv *KVServer) PutAppend(args *RpcArgs, reply *RpcReply) {
	// Your code here.
	// kv.mu.Lock()
	// DPrintf("server[%d] PutAppend, ClientId= %d, MsgId= %d", kv.me, args.ClientId, args.MsgId)
	// kv.mu.Unlock()

	log := Op{
		Cmd : args.Cmd,
		Key : args.Key,
		Value : args.Value,
		ClientId : args.ClientId,
		MsgId : args.MsgId,
	}
	kv.commitLog(args, reply, &log)
}

func (kv *KVServer) commitLog(args *RpcArgs, reply *RpcReply, log *Op) {
	raftIndex, _, isLeader := kv.rf.Start(*log)
	if !isLeader {
		DPrintf("server[%d] not leader", kv.me)

		reply.Info = NotLeader
		return
	}

	kv.mu.Lock()
	DPrintf("server[%d] PutAppend, ClientId= %d, MsgId= %d raftIndex = %d", kv.me, args.ClientId, args.MsgId, raftIndex)

	waitCh, IsExist := kv.waitCh[raftIndex]
	if !IsExist {
		kv.waitCh[raftIndex] = make (chan Op, 1)
		waitCh = kv.waitCh[raftIndex]
	}
	DPrintf("server[%d] open waitCh for raftIndex = %d", kv.me, raftIndex)
	kv.mu.Unlock()

	// wait for channel reply
	select {
	case <- time.After(time.Millisecond * 2000):
		// timeout， need to resend
		reply.Info = Timeout
		
	case OpRes := <- waitCh:
		// log commit success
		DPrintf("server[%d] recv from waitCh", kv.me)

		reply.Info = Success
		kv.executeCmd(&OpRes, reply)
	}

	kv.mu.Lock()
	DPrintf("server[%d] delete waitCh for raftIndex = %d", kv.me, raftIndex)
	delete(kv.waitCh, raftIndex)
	kv.mu.Unlock()
}

func (kv *KVServer) executeCmd(operation *Op, reply *RpcReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	LastIndex, IsExist := kv.LastCmdIndex[operation.ClientId]
	// if !IsExist {
	// 	kv.LastCmdIndex[operation.ClientId] = operation.MsgId
	// 	LastIndex = operation.MsgId
	// } 
	
	if IsExist && operation.MsgId <= LastIndex {
		// Duplicate cmd
		DPrintf("server[%d] find dup command, MsgId = %d", kv.me, operation.MsgId)

		return
	}

	// execute cmd
	if operation.Cmd == "Get" {
		res, IsExist := kv.kvDB[operation.Key]
		if !IsExist {
			reply.Value = ""
		} else {
			reply.Value = res
		}
	} else if operation.Cmd == "Put" {
		kv.kvDB[operation.Key] = operation.Value
		reply.Value = ""
	} else if operation.Cmd == "Append" {
		kv.kvDB[operation.Key] += operation.Value
		reply.Value = ""
	}

	// update lastIndex
	kv.LastCmdIndex[operation.ClientId] = operation.MsgId

}
// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// You may need initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.

	kv.kvDB = make(map[string]string)	
	kv.waitCh = make(map[int]chan Op)
	kv.LastCmdIndex = make(map[int64]int)
	go kv.ApplyToChannel()

	return kv
}
