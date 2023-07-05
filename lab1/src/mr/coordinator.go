package mr

import "log"
import "net"
import "os"
import "net/rpc"
import "net/http"
import "sync"
import "fmt"
import "strconv"
import "time"

var TIMEOUT int64 = 5;

type Coordinator struct {
	coorMutex           sync.Mutex
	nReduce             int
	nMap                int
	mapTasks            []Task
	reduceTasks         []Task
	state               int // MASTER_INIT;MAP_FINISHED;REDUCE_FINISHED
	mapTaskFinishNum    int
	reduceTaskFinishNum int
	timerSortList       TimerList
}


func (c *Coordinator) CoorInit(files []string, nReduce int) {
	c.state = -1;
	// 1. read input file 
	// 2. split to  task
	nMap := len(files)
	for id, filename := range files {
		// create Task
		task := Task{}
		// MAP_TASK
		task.State = 0
		task.InputFileName = filename
		task.Id = id
		task.TaskType = 0
		task.NMap = nMap
		task.NReduce = nReduce
		c.mapTasks = append(c.mapTasks,task)
	}
	
	c.nReduce = nReduce
	c.nMap = nMap
	c.mapTaskFinishNum = 0
	c.reduceTaskFinishNum = 0
	c.state = 0;

	c.GenReduceTask()
}

func (c *Coordinator) SendTask(WorkerPid int, replyTask *Task) error{
	// mutex 保护
	c.coorMutex.Lock()
	fmt.Println("-----SendTask-----")
	if(c.state == 0) {
		// send Map task
		// 查找超时Task
		c.timerSortList.check(TIMEOUT)
		// 遍历所有task的状态
		// 选择状态为INIT的
		i := 0
		for i = 0; i<len(c.mapTasks); i++ {
			if c.mapTasks[i].State == 0 {
				// send task to client
				c.mapTasks[i].WorkerPid = WorkerPid
				c.mapTasks[i].timer = Timer{
					time : time.Now().Unix(),
					pTask: &c.mapTasks[i]}

				// 深拷贝task对象??
				// Clone(&c.mapTasks[i], replyTask)
				*replyTask = c.mapTasks[i]

				c.timerSortList.addTimer(&c.mapTasks[i].timer)
				c.mapTasks[i].State = 1
				break
			} 
		}

		fmt.Println(" send map task ",i)
		if i == len(c.mapTasks) {
			replyTask.TaskReply = "continue"
			fmt.Println("all mapTask is processing now")
		} else {
			replyTask.TaskReply = "getTask"
		}
	} else if (c.state == 1) {

		c.timerSortList.check(TIMEOUT)
		// send Reduce task
		i := 0
		for i = 0; i<len(c.reduceTasks); i++ {
			if c.reduceTasks[i].State == 0 {
				// send task to client
				c.reduceTasks[i].WorkerPid = WorkerPid
				c.reduceTasks[i].timer = Timer{
					time : time.Now().Unix(),
					pTask: &c.reduceTasks[i]}
				*replyTask = c.reduceTasks[i]
				// Clone(&c.reduceTasks[i], replyTask)

				c.timerSortList.addTimer(&c.reduceTasks[i].timer)
				c.reduceTasks[i].State = 1
				break
			} 
		}
		// send null to client
		fmt.Println(" send reduce task ",replyTask.Id)

		if i== len(c.reduceTasks){
			replyTask.TaskReply = "continue"
			fmt.Println("all reduceTask is processing now")
		} else {
			replyTask.TaskReply = "getTask"
		}
	} else {
		// reduce task is over
		replyTask.TaskReply = "break"
		fmt.Println("all task is finish")
	}
	defer c.coorMutex.Unlock()

	return nil
}
func (c *Coordinator) GenReduceTask(){
	// map task produce nMap * nReduce files
	// one reduce task need to handle nMap files
	// mr-out-i-j (i->nMap, j->nReduce)
	// mr-out-0-reduceTaskId --> mr-out-(nMap-1)-reduceTaskId
	for i := 0; i < c.nReduce; i++{
		task := Task{}
		task.State = 0
		task.InputFileName = "worker_output/mr-out-"
		task.Id = i
		task.OutputFileName = "mr-out-0-" + strconv.Itoa(i)
		task.TaskType = 1
		task.NReduce = c.nReduce
		task.NMap = c.nMap

		c.reduceTasks = append(c.reduceTasks,task)
	}
}

func (c *Coordinator) TaskRespone(args *TaskResponeArgs, taskChanged *bool) error{
	WorkerPid := args.Pid
	id := args.Id
	c.coorMutex.Lock()
	fmt.Println("-----TaskRespone-----")
	if(c.state == 0) {
		// recive map task done
		fmt.Println("task id ",id," --- ",c.mapTaskFinishNum,"--", c.nMap)
		
		// 1. check pid
		if c.mapTasks[id].WorkerPid != WorkerPid {
			fmt.Println("task ",id," is not belong to ", WorkerPid)
			*taskChanged = true
		} else {
			// 2. submit
			c.mapTasks[id].State = 2
			fmt.Println("---", c.mapTasks[id].timer.time)

			c.timerSortList.delTimer(&(c.mapTasks[id].timer))

			c.mapTaskFinishNum ++;
			if(c.mapTaskFinishNum == c.nMap) {
				// MAP_FINISHED;
				c.state = 1
				// delete all the timer
				c.timerSortList.check(0)
			}
			*taskChanged = false
		}

	} else if (c.state == 1) {
		// recive reduce task done
		fmt.Println("task id ",id)
		if c.reduceTasks[id].WorkerPid != WorkerPid {
			fmt.Println("task ",id," is not belong to ", WorkerPid)
			*taskChanged = true
		} else {
			c.reduceTasks[id].State = 2
			c.timerSortList.delTimer(&(c.reduceTasks[id].timer))

			c.reduceTaskFinishNum ++;
			if(c.reduceTaskFinishNum == c.nReduce){
				// REDUCE_FINISHED
				c.state = 2
				c.timerSortList.check(0)
			}
			*taskChanged = false
		}
	}
	defer c.coorMutex.Unlock()
	return nil
}

// func Clone(orgin *Task, dest *Task){
// 	dest.State = orgin.State           
// 	dest.InputFileName = orgin.InputFileName  
// 	dest.Id = orgin.Id             
// 	dest.OutputFileName = orgin.OutputFileName 
// 	dest.TaskType = orgin.TaskType        
// 	dest.NReduce = orgin.NReduce        
// 	dest.NMap = orgin.NMap           
// 	dest.timer = orgin.timer
// 	dest.WorkerPid = orgin.WorkerPid
// }

//
// start a thread that listens for RPCs from worker.go
//
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
//
func (c *Coordinator) Done() bool {
	ret := false

	// Your code here.
	if(c.state == 2) {
		ret = true
	}

	return ret
}


//
// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{}

	// Your code here.
	// generate maptask and reduce task
	c.CoorInit(files, nReduce)

	// start an http server
	// wait for client to take task
	c.server()
	return &c
}
