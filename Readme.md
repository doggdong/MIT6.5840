# Readme

## Lab2A：实现leader选举算法

任务

- 设置节点三种状态，**leader，candidate，follower**之间转换关系（pic）
- 设置follower和candidate超时时间
- 设置投票规则
  1. candidate's term is lower, not vote
  2. votedFor == -1 or candidate's log is more update, vote
  3. 其他情况都不投票
- leader实现周期性发送心跳

实现

- 状态转换利用协程实现

  ```
  go beFollower()
  go beCandidate()
  go beLeader()
  ```

- 超时时间设置

  这两个超时时间是有关系的，follower的超时时间理解为：（follower发送心跳应答 ， leader回复心跳） 此过程的最长时间，为了保证一轮心跳期间不会超时；candidate的超时时间理解为：（candidate发送投票请求，follower回复投票请求）此过程的最长时间；两个超时时间最大值都由两次RPC的时间决定。

  ```cpp
  timeDuration := 300 + (rand.Int63() % 150)
  rf.timeReset = time.Now().UnixMilli()
  
  for {
  	// ...... //
      
      if time.Now().UnixMilli() - rf.timeReset > timeDuration{
          // times out, starts election
          // term increase by itself
          rf.state = Candidate
          rf.term = rf.term+1
          rf.votedFor = rf.me
  
          go rf.beCandidate(rf.term)
          rf.mu.Unlock()
          return
      }
      
      // ...... //
  }
  ```

- 设置投票规则

  follower如下， candidate和leader也需要处理投票请求，和follower类似

  ```cpp
  if rf.state == Follower {
      if args.Term >= rf.term {
          if  (args.Term > rf.term || rf.votedFor == -1) && rf.CompareTo(args.LastLogIndex, args.LastLogTerm){
              rf.term = args.Term
                  reply.Term = rf.term
                  rf.votedFor = args.Id
                  rf.stateReset = true
  
                  reply.GetVote = true
          } else  {
              // 不投票
              rf.term = args.Term
                  reply.Term = rf.term
                  reply.GetVote = false
          }
      } else {
          // args.Term < rf.term
          // 不投票, 继续做Follower
          reply.Term = rf.term
              reply.GetVote = false
              rf.stateReset = false
      }
  }
  ```



注意事项

- 调用rpc期间不能加锁, 如果一个rpc因为网络原因没能返回, 则会导致锁无法释放
  - A sendRpc to B 意味着B会加锁处理rpc
  - 假如sendRPC加锁, B sendRpc to A, 导致B处理A发来的Rpc过程会被挂起等待
  - A B同时向对方sendRPC, A B 全都acquire lock, rpc发送到之后, B A就无法处理RPC了(处理RPC需要加锁)
  - 所以, sendRpC万万不能加锁
- 选举超时时间只能在下面三种情况下重置：
  - 收到现任 Leader 的心跳请求，如果 AppendEntries 请求参数的任期是过期的(args.Term < currentTerm)，不能重置;
  - 节点开始了一次选举;
  - 节点投票给了别的节点(没投的话也不能重置)（原因： 如果不投票也重置，会导致一个candidate一直处于候选失败状态）
- candidate发送的投票请求可能会延迟到达，所以candidate收到的投票可能是过期的
  - candidate sendRPC时， 返回值reply记录发送投票请求时的term
  - candidate sendRPC结束后，检查收到的reply.term 是否来自之前的任期
- 网络分区（脑裂）
  - 选举机制保证了两个leader一定处于不同的term
  - 分区恢复后必定有一个leader被打回follower状态



## Lab2B：实现日志一致性复制

任务：

- 每个节点新增数组成员，代表log
- 实现Start() 接口，只有leader能主动向log中append元素
- leader心跳中携带要提交的log
  - leader通过心跳和follower比较log，判断follower需要删除log，需要append log，已经需要从哪里append



实现

注意

- 发送心跳的周期论文上是50-100ms， 但千万不能按照这个设置，
  - follower端没来得及处理回复，leader端就超时重发心跳
  - 导致leader发送大量重复的心跳
  - 后期follower先处理大量过时的心跳，逐个回复后，才处理到正确的心跳（！解决这一问题对lab3A的 speedTest很有帮助）
  - 恶性循环，leader重发的越多，follower处理的越慢，follower一次心跳周期内没回应，leader会继续接着重发

## Lab2C

## Lab2D

## Lab3A: 实现key-value服务器

任务

- 编写client.go 和 server.go
- client中定义put append操作，通过rpc发送给server端
- server中将client的命令请求包装为log，提交给Lab2的raft对象
- server开启协程，监听raft集群，如果集群提交一个log，server开始执行client的命令，并像client通知命令执行成功

实现

- 每收到一个client命令，对他产生唯一的index，并阻塞等待此index的log被raft集群提交

注意

- 命令被覆盖：index处的log被提交，还需要检查term  (原因：index+term才能唯一标定一个log，假如遇到figure8的情况，index处的log会被其他term的log替换，导致虽然日志被覆盖而未提交，但server的index处却收到了响应)
- 命令去重：server执行命令需要判断命令是否已经执行过（原因： client有超时重发机制，一条命令可能会被client发送多次，也就导致raft集群也提交多次）
  - 首先client端为每个命令都生成了唯一的id
  - server端为每个client维护一个lastCmdIndex，如果提交log对应的id小于lastCmdIndex，则说明命令重复

# 问题记录

1. client
   client 会面向多个server， 需要判断哪个server是leader
   client端的get append 负责远程rpc调用server端的get append， 所以主要还是写server端的功能
   同一个client可能多次像server发出同一个命令，也有可能一个命令由于网络原因被client超时重传多次
   多个client也可也能像server发出同一个命令
   根据以上的问题，需要根据client的id和命令执行顺序来给命令设置唯一的id
2. server
   1. server端对应raft集群中的一个节点，并且初始化一个applyCh，由前面的lab可知，server需要通过这个channel来接收集群commit的log
   2. server如何执行 get append
      1. 首先判断自己是否还是leader
      2. 将get append命令 通过 raft的 Start()接口提交给leader
      3. 等待命令被commit， 超时后未commit就重新发送
   3. 如何判断命令已经被commit
      1. 从leader的applyCh中接收到的log，都是已经commit的
      2. 开启一个go协程， 从applyCh中读，并且可以读出该log是哪个client发来的
   4. go协程读取到的log，如何返回给不同的client
      1. 假如是get命令， 返回get结果到client，是server中get函数的任务
      2. 所以在go协程中，只需要将get结果通过channel发给server的get函数就可以
      3. 因为
         1. 同一个client会多次调用同一个server的get函数
         2. 多个不同的client会一起调用同一个server的get函数
            所以每个命令都需要一个channel来接收
   5. 实现方法
      1. server中定义哈希表，命令Id -> channel的映射
      2. 每像server中Start()一个log， 就定义一个channel等待返回，并用哈希表记录
      3. channel接收到返回，证明命令已经提交，server执行命令，返回结果给client
      4. 返回后，删除哈希表对应的索引，防止空间无限膨胀 
   6. leader更换时，需要对每个client的lastIndex重新初始化
      1. 初始化为当前收到的请求是否会造成同一命令重复执行
         1. 过程，命令A首先在leader1上被提交并执行，则客户端不会再重发
         2. 客户端重发的情况

## 问题

#### 	1. lastLog is more update的节点， 会赢得选票， 此节点的commitIndex会不会比当前leader小呢？

​		如果这种情况发送，就会导致二次commit
​		1. 图八对应的情况
​			情况1： 一个节点124，并且12已经提交，并不妨碍一个13节点当leader
​			情况2： 一个13节点都commit，并不妨碍124 赢得选票
​		2. 解决办法 --> 只能commit当前任期
​			1. 124节点想提交12就必须先提交4， 这样13无法当leader
​			2. 13节点全部commit，说明当前任期一定 ==3, 不会出现 log more update的节点
​		

2. #### leader一直在发送心跳,preIndex为倒数第二位,但是follower回复一直没有确认倒数第一位的匹配

   ​	1. 首先检查leader发送心跳是否携带了倒数第一位的信息
   ​	2. 检查follower的返回机制
   ​	3. 描述问题
   ​		1. leader一直在发送已经同步过的消息, follower的确认回复迟迟不到, 导致leader重发相同的心跳
   ​		2. follower一直在回复旧消息的确认信息,先回复旧消息,后回复新消息
   ​		3. 真正原因是, follower忙于处理成堆的旧消息,新消息处理前,leader发送了大量新消息,导致下一次日志提交follower还需要处理这批消息之后才能处理新log
   ​			1. start后立马发送心跳
   ​			2. 心跳间隔调大,否则follower处理不过来,
   ​			3. 接收心跳只处理大index的回复,避免因为旧心跳导致新心跳重新append log
   ​		4. 测试
   ​			1. 1000 -> 36s, 900 -> 24s, 800 ->16s, 700 ->12s, 500 -> 6s 
   ​			2. 最后200个处理速度为 12s/100个

3. #### 解决方案

    	1. follower端设置队列,每次只处理队尾的消息 --> 无效, 会导致真正前面的消息失效

   3. kv 锁和 rf 锁会冲突??
   4. 多次start返回的index相同
      1. 为了降低处理时间,start后立马发送心跳,是通过channel完成的
      2. 朝channel发消息需要解开锁,导致前后数据不一致

   5. 定义waitCh时提交的log和最终接收到waitCh中的log不一样
      1. 当一个log被append, 就记录被append的index, 然后开一个waitCh等待raft apply这个index
      2. 当发生网络分区时, 一个位置上append的,和最终commit的可能不是同一个log
      3. 问题描述
         1. cmd1 -> leader1, append 到 index, 开启监听index的waitCh, 随后leader1 发生网络分区
         2. cmd2 -> leader2, 网络分区恢复, leader1的index被cmd2覆盖, apply提交,唤醒leader1的waitCh
         3. 导致leader1监听的是cmd1, 收到的却是cmd2, 错误将reply.Success返回给 cmd1的客户端
         4. 这样即使分区恢复了, cmd2也不再会重发消息了
      4. 解决方法:
         1. 判断waitCh收到的term与open waitch时是否一致
   6. for循环中apply会有问题??
      1. 修改先拷贝后在赋值
      2. 修改先深拷贝出后,再解锁,再for循环
   7. defer mu.Unlock() 期间 mu.Unlock会造成重复unlock
   8. 两个地方去重
      1. leader接收到心跳回复时, 
         1. follower如果匹配成功, nextIndex 不能变小,防止follower收到旧心跳后返回一个很小的matchIndex,导致nextIndex也变很小
         2. follower如果匹配失败, nextIndex 不能变大
      2. follower收到心跳时, 如果心跳携带的log已经被commit,就当做旧心跳,
   9. apply snapshot和 apply log之间有冲突
      10 apply 时需要解开锁,但apply的同时又需要更新rf.lastApply
      1. 先更新 rf.lastApply, 再解开锁apply,
      2. 由于apply是心跳触发的, 同时收到两个心跳会同时执行两个apply,导致重复apply
      3. 把心跳触发的apply更改为, 单独一个协程内apply
         11 如何保证 index+1匹配 则 index之前的都匹配呢?
   10. 2c figure8 unliable 会超时到10分钟
       1. debug发现问题在start时阻塞
          原因
       2. start通过channel通知leader的心跳发送模块，而channel是有大小限制的，正常情况下一个channelpush另一端立马会消耗一个channel并发送心跳
       3. 特殊情况是往channel放值的时候，突然leader变成follower，心跳发送模块被关闭，就没有协程可以消耗channel了，start就被阻塞了 