package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import "os"
import "strconv"
import "time"
import "fmt"
//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

type TaskResponeArgs struct {
	Pid int
	Id  int
}
// Add your RPC definitions here.
type Task struct {
	TaskReply      string
	State          int // TASK_INIT;TASK_PROCESSING;TASK_DONE
	InputFileName  string
	Id             int
	OutputFileName string
	TaskType       int // MAP_TASK;REDUCE_TASK
	NReduce        int
	NMap           int
	StartTime      int64
	timer		   Timer
	WorkerPid	   int
}

// 按照time从大到小排序
type TimerList struct {
	head		  *Timer
	tail		  *Timer
}

type Timer struct{
	time		 int64
	prev		 *Timer
	next		 *Timer
	pTask        *Task
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/5840-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}

// 大的放在前面
func (tl *TimerList) addTimer(newTime *Timer) {
	
	if tl.head == nil{
		// dqueue is empty
		tl.head = newTime
		tl.tail = newTime
	} else if newTime.time >= tl.head.time {
		// insert as head
		tl.head.prev =  newTime
		newTime.next = tl.head
		tl.head = newTime
	} else {
		tmp := tl.head
		for tmp!=nil && newTime.time < tmp.time {
			tmp = tmp.next
		}

		if tmp!=nil {
			prev := tmp.prev;
			prev.next = newTime
			newTime.next = tmp
			tmp.prev = newTime
			newTime.prev = prev
		} else {
			newTime.prev = tl.tail
			tl.tail.next = newTime
			tl.tail = newTime
		}

	}
}

// 从前往后遍历, 遇到足够旧的就开始删除
func (tl *TimerList) check(deltaTime int64) {
	fmt.Println("------ check ------")
	currTime := time.Now().Unix()
	
	tmp := tl.head
	for tmp!=nil && tmp.time + deltaTime >= currTime {
		tmp = tmp.next
	}


	for tmp != nil {
		fmt.Println("timer has delay ", tmp.pTask.Id)
		next := tmp.next
		tl.delTimer(tmp)
		tmp = next
	}

}
func (tl *TimerList) delTimer(t *Timer) {
	fmt.Println("delete timer ", t.pTask.Id)

	// 1. 从链表中删除timer
	if t == tl.head && t == tl.tail{
		tl.head = nil
		tl.tail = nil
	} else if t == tl.head {
		t.next.prev = nil
		tl.head = t.next
	} else if t == tl.tail {
		t.prev.next = nil
		tl.tail = t.prev
	} else {
		t.prev.next = t.next
		t.next.prev = t.prev
	}
	// 2. 将Task状态位还原
	if t.pTask.State == 0{
		fmt.Println("impossble")
	} else if t.pTask.State == 1 {
		fmt.Println("recovery task ", t.pTask.Id)
		t.pTask.State = 0
		t.pTask.WorkerPid = 0
	} else if t.pTask.State == 2 {
		fmt.Println("task over ", t.pTask.Id)
	}

}
