package mr

import "fmt"
import "log"
import "net/rpc"
import "hash/fnv"
import "os"
import "io/ioutil"
import "strconv"
import "encoding/json"
import "sort"

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}


//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {

	pid := os.Getpid()
	fmt.Printf("worker %d start!\n",pid)
	for true {
		task := Task{}
		// 1. ask coordinator for a task
		AskForTask(pid, &task)
		fmt.Printf("---- %d %d ----reply: %s \n",task.Id, task.TaskType, task.TaskReply)

		if task.TaskReply == "continue" {
			fmt.Println("this worker need no job")
			continue
		} else if task.TaskReply == "break"{
			fmt.Println("this worker need to shutdown")
			break
		}

		ExecuteTask(pid, &task, mapf, reducef)
		fmt.Printf("task over %d %d\n",task.Id, task.TaskType)

		// 2. tell the coordinator work is over
		taskChanged := false
		args := TaskResponeArgs{}
		args.Pid = pid
		args.Id = task.Id
		FinishTask(&args, &taskChanged)
		fmt.Printf("finish over %d %d\n",task.Id, task.TaskType)
		
		if !taskChanged {
			// submit 
			SubmitResult(&task)
		} else {
			// delete temp file
			DeleteResult(&task)
		}
	}

}

func AskForTask(pid int, task interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call("Coordinator.SendTask", pid, task)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}

func ExecuteTask(pid int, task *Task, mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {
	
	if(task.TaskType == 0){
		// map task
		// 1. read string from inputfile
		file, _ := os.Open(task.InputFileName)
		content, _ := ioutil.ReadAll(file)

		// 2. execute map func
		intermediate := []KeyValue{}
		kva := mapf(task.InputFileName, string(content))
		intermediate = append(intermediate, kva...)
		
		hashedKva := make(map[int][]KeyValue)
		fmt.Println("---++-",task.NReduce)
		// 3. save res to nReduce files
		for _, kv := range intermediate {
			hashed := ihash(kv.Key) % task.NReduce
			hashedKva[hashed] = append(hashedKva[hashed], kv)
		}

		os.MkdirAll("worker_output", os.ModePerm)
		for i:=0; i<task.NReduce; i++ {
			ofile, _ := os.Create("worker_output/mr-out-"+strconv.Itoa(task.Id)+"-"+strconv.Itoa(i)+"-temp")

			// // sort inside hashedKva[i] 
			// sort.Sort(ByKey(hashedKva[i]))
			
			// save to the file
			enc := json.NewEncoder(ofile)
			for _, kv := range hashedKva[i] {
				err := enc.Encode(&kv)
				if err!=nil {
					fmt.Println("json Encode error")
				}
			}
		}
		file.Close()
		
	} else if task.TaskType == 1 {
		// reduce task
		// 1. get nMap file list
		intermediate := []KeyValue{}
		for i:=0; i<task.NMap; i++ {
			filename := task.InputFileName + strconv.Itoa(i) + "-" +strconv.Itoa(task.Id)
			file, _ := os.Open(filename)
			dec := json.NewDecoder(file)
			for {
				var kv KeyValue
				if err := dec.Decode(&kv); err != nil {
					break
				}
				intermediate = append(intermediate, kv)
			}
			file.Close()
		}
		// sort inside
		sort.Sort(ByKey(intermediate))

		i := 0
		ofile, _ := os.Create(task.OutputFileName + "-temp")
		for i < len(intermediate) {
			j := i + 1
			for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
				j++
			}
			values := []string{}
			for k := i; k < j; k++ {
				values = append(values, intermediate[k].Value)
			}
			output := reducef(intermediate[i].Key, values)
	
			// this is the correct format for each line of Reduce output.
			fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)
	
			i = j
		}

	}
}

func FinishTask(args interface{}, taskChanged *bool) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call("Coordinator.TaskRespone", args, taskChanged)
	// err = c.Call("Coordinator.TaskRespone", args, taskChanged)
	if err == nil {
		return true
	}

	fmt.Println("call error: ", err)
	return false
}

// delete temp file
func DeleteResult(task *Task){
	if task.TaskType == 0 {
		// one map task produce NReduce output file
		for i:=0; i<task.NReduce; i++{
			os.Remove("worker_output/mr-out-"+strconv.Itoa(task.Id)+"-"+strconv.Itoa(i)+"-temp")
		}
	} else if task.TaskType == 1 {
		// one reduce task produce one output
		os.Remove(task.OutputFileName + "-temp")
	}
}

// rename temp to final file
func SubmitResult(task *Task){
	if task.TaskType == 0 {
		// one map task produce NReduce output file
		for i:=0; i<task.NReduce; i++{
			os.Rename("worker_output/mr-out-"+strconv.Itoa(task.Id)+"-"+strconv.Itoa(i)+"-temp", 
			"worker_output/mr-out-"+strconv.Itoa(task.Id)+"-"+strconv.Itoa(i))
		}
	} else if task.TaskType == 1 {
		// one reduce task produce one output
		os.Rename(task.OutputFileName + "-temp", task.OutputFileName )
	}
}
