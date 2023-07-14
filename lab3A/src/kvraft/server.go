package kvraft

import (
	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
	"log"
	"sync"
	"sync/atomic"

	"time"
	// "fmt"
)

// const Debug = true
const Debug = false
func DPrintf(format string, a ...interface{}) (n int, err error) {
	log.SetFlags(log.Lmicroseconds)
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
	Term         int
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
	for !kv.killed() {
		msg := <- kv.applyCh
		kv.mu.Lock()

		if msg.CommandValid {
			operation := msg.Command.(Op)
			operation.Term = msg.CommandTerm

			// fmt.Printf("server[%d] ApplyToChannel recv, Raftindex = %d, MsgIndex = %d, key= %v value= %v\n", kv.me, msg.CommandIndex, operation.MsgId, operation.Key, operation.Value)
			kv.executeCmd(operation)
	
			// check if the wait channel is exist
			// msg.commandIndex is same as raftIndex
			// DPrintf("server[%d] ApplyToChannel recv, Raftindex = %d, MsgIndex = %d, key= %v value= %v", kv.me, msg.CommandIndex, (msg.Command.(Op)).MsgId, (msg.Command.(Op)).Key, (msg.Command.(Op)).Value)
			DPrintf("server[%d] ApplyToChannel recv, Raftindex = %d, MsgIndex = %d, key= %v value= %v", kv.me, msg.CommandIndex, operation.MsgId, operation.Key, operation.Value)
			waitCh, IsExist := kv.waitCh[msg.CommandIndex]
			if !IsExist{
				// waitCh has been delete
				DPrintf("server[%d] channel wait for, Raftindex = %d, has been delete", kv.me, msg.CommandIndex)
			} else {
				// send to waitCh
				DPrintf("server[%d] ApplyToChannel recv, send to waitCh ", kv.me)
				waitCh <- operation
			}
		} else if msg.SnapshotValid {
	
		}
		kv.mu.Unlock()

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
	kv.commitLog(args, reply, log)
}

func (kv *KVServer) PutAppend(args *RpcArgs, reply *RpcReply) {
	// Your code here.
	kv.mu.Lock()
	DPrintf("server[%d] PutAppend, ClientId= %d, MsgId= %d", kv.me, args.ClientId, args.MsgId)
	kv.mu.Unlock()

	log := Op{
		Cmd : args.Cmd,
		Key : args.Key,
		Value : args.Value,
		ClientId : args.ClientId,
		MsgId : args.MsgId,
	}
	kv.commitLog(args, reply, log)
}

func (kv *KVServer) commitLog(args *RpcArgs, reply *RpcReply, log Op) {
	// check dup
	
	// kv.mu.Lock()
	// DPrintf("1")
	// if kv.checkDup(log) {
	// 	kv.mu.Unlock()
	// 	reply.Info = Success
	// 	kv.getReply(log, reply)
	// 	return
	// }
	// kv.mu.Unlock()
	
	raftIndex, currentTerm, isLeader := kv.rf.Start(log)

	kv.mu.Lock()
	if !isLeader {
		DPrintf("server[%d] not leader", kv.me)
		
		reply.Info = NotLeader
		kv.mu.Unlock()
		return
	}

	DPrintf("server[%d] , ClientId= %d, MsgId= %d append log", kv.me, args.ClientId, args.MsgId)
	// DPrintf("server[%d] PutAppend, ClientId= %d, MsgId= %d raftIndex = %d", kv.me, args.ClientId, args.MsgId, raftIndex)

	waitCh, IsExist := kv.waitCh[raftIndex]
	if !IsExist {
		kv.waitCh[raftIndex] = make (chan Op, 1)
		waitCh = kv.waitCh[raftIndex]
	}
	DPrintf("server[%d] open waitCh for raftIndex = %d cmd= %v key= %v value= %v", kv.me, raftIndex, log.Cmd, log.Key, log.Value)
	kv.mu.Unlock()

	// wait for channel reply
	select {
	case <- time.After(time.Millisecond * 2000):
		// timeout， need to resend
		DPrintf("server[%d] wait raftIndex = %d timeout change leader", kv.me, raftIndex)

		reply.Info = NotLeader
		
	case OpRes := <- waitCh:
		// log commit success
		if currentTerm != OpRes.Term {
			// 提交时的Term和最终log记录的Term不一致,说明不是同一个log
			reply.Info = NotLeader
		} else {
			DPrintf("server[%d] recv from waitCh, raftIndex = %d cmd= %v key= %v value= %v", kv.me, raftIndex, OpRes.Cmd, OpRes.Key, OpRes.Value)
			reply.Info = Success
			kv.mu.Lock()
			kv.getReply(OpRes, reply)
			kv.mu.Unlock()
		}
	}

	kv.mu.Lock()
	DPrintf("server[%d] delete waitCh for raftIndex = %d", kv.me, raftIndex)
	delete(kv.waitCh, raftIndex)
	kv.mu.Unlock()
}

func (kv *KVServer) checkDup(operation *Op) bool {
	LastIndex, IsExist := kv.LastCmdIndex[operation.ClientId]
	
	if IsExist && operation.MsgId <= LastIndex {
		// Duplicate cmd
		DPrintf("server[%d] find dup command, MsgId = %d", kv.me, operation.MsgId)

		return true
	}
	return false
}
func (kv *KVServer) getReply(operation Op, reply *RpcReply) {

	// fmt.Println("")
	if operation.Cmd == "Get" {
		res, IsExist := kv.kvDB[operation.Key]
		if !IsExist {
			reply.Value = ""
		} else {
			reply.Value = res
		}
	} else {
		reply.Value = ""
	}
	DPrintf("server[%d] get reply cmd= %v key= %v value= %v res= %v", kv.me, operation.Cmd, operation.Key, operation.Value, reply.Value)

}
func (kv *KVServer) executeCmd(operation Op) {
	
	// operation := command.(Op)
	DPrintf("server[%d] execute %v key= %v value= %v", kv.me, operation.Cmd, operation.Key, operation.Value)
	if kv.checkDup(&operation) {
		return
	}

	// execute cmd
	if operation.Cmd == "Get" {

	} else if operation.Cmd == "Put" {
		kv.kvDB[operation.Key] = operation.Value


	} else if operation.Cmd == "Append" {
		kv.kvDB[operation.Key] += operation.Value

	}

	// update lastIndex
	lastIndex, IsExist := kv.LastCmdIndex[operation.ClientId]
	if !IsExist || lastIndex < operation.MsgId{
		kv.LastCmdIndex[operation.ClientId] = operation.MsgId
	}

	kv.showCurMap(operation)
}

func (kv *KVServer) showCurMap(operation Op) {
	DPrintf("server[%d] execute over %v key= %v value= %v", kv.me, operation.Cmd, operation.Key, operation.Value)
	// fmt.Printf("server[%d] execute over, cmd = %v key= %v value= %v\n", kv.me, operation.Cmd, operation.Key, operation.Value)
	// fmt.Println(kv.kvDB)
	// if _, ok := kv.rf.GetState(); ok{
	// }
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
