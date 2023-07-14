package kvraft

import "6.5840/labrpc"
import "crypto/rand"
import "math/big"
// import "unsafe"
// import "fmt"

const(
	NotLeader = iota
	NoSuchKey

	Success
	Timeout
	Duplicate

)

type RpcArgs struct {
	Cmd        string
	Key        string
	Value      string
	ClientId   int64
	MsgId      int
}
type RpcReply struct {
	Info      int
	Value     string
}

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	leaderId  int
	ClientId  int64 
	MsgId     int
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	ck.ClientId = nrand()
	ck.MsgId = 0
	ck.leaderId = 0
	// You'll have to add code here.


	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {

	// You will have to modify this function.
	// send rpc to leader, to commit a log
	// DPrintf("client[%d] Get, MsgId= %d", ck.ClientId, ck.MsgId)
	// fmt.Printf("client[%d] Get %v\n", ck.ClientId, key)

	args := RpcArgs{
		Cmd : "Get",
		Key : key,
		Value : "",
		ClientId : ck.ClientId,
		MsgId : ck.MsgId,
	}
	
	for {
		reply := RpcReply{}

		ok := ck.servers[ck.leaderId].Call("KVServer.Get", &args, &reply)
	
		if !ok || reply.Info == NotLeader {
			// resend rpc
			// num := nrand() % int64(len(ck.servers))
			// ck.leaderId = *(*int)(unsafe.Pointer(&num))
			ck.leaderId = (ck.leaderId + 1) % len(ck.servers)
			DPrintf("client[%d] change leader resend", ck.ClientId)

			continue
		} else if reply.Info == Timeout {
			continue
		} else if reply.Info == Success{
			DPrintf("client[%d] Get %v, success, value= %v", ck.ClientId, key, reply.Value)
			// fmt.Printf("client[%d] Get %v, success, value= %v\n", ck.ClientId, key, reply.Value)

			ck.MsgId++
			return reply.Value
		} 

	}

}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	DPrintf("client[%d] PutAppend, key=%v, value=%v, MsgId= %d", ck.ClientId, key, value, ck.MsgId)

	args := RpcArgs{
		Cmd : op,
		Key : key,
		Value : value,
		ClientId : ck.ClientId,
		MsgId : ck.MsgId,
	}
	
	
	for {
		reply := RpcReply{}

		ok := ck.servers[ck.leaderId].Call("KVServer.PutAppend", &args, &reply)
	
		if !ok || reply.Info == NotLeader {
			// resend rpc
			// num := nrand() % int64(len(ck.servers))
			// ck.leaderId = *(*int)(unsafe.Pointer(&num))
			ck.leaderId = (ck.leaderId + 1) % len(ck.servers)

			// DPrintf("client[%d] PutAppend, MsgId= %d, change Leader, resend", ck.ClientId, ck.MsgId)
			DPrintf("client[%d] putAppend, cmd= %v, key= %v, value= %v resend", ck.ClientId, op,key, value)

			continue
		} else if reply.Info == Timeout {
			DPrintf("client[%d] PutAppend, MsgId= %d, timeout, resend", ck.ClientId, ck.MsgId)

			continue
		} else if reply.Info == Success{
			DPrintf("client[%d] PutAppend, cmd= %v, key= %v, value= %v MsgId= %d, reply success", ck.ClientId, op, key, value, ck.MsgId)

			ck.MsgId++
			return
		} 

	}

}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
