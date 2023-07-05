import os
# for i in range(300):
#     print("TestBackup",i)

#     os.system("echo \'========= %d =========\n\' >> outputlog2.txt " %i)
#     # os.system("go  test -race -run TestFailNoAgree2B  >> outputlog2.txt ")
#     os.system("go  test -race -run TestBackup2B  >> outputlog2.txt ")
#     os.system("echo \'========= %d over =========\n\' >> outputlog2.txt " %i)

for i in range(500):
    print("2B:" ,i)

    os.system("echo \'========= %d =========\n\' >> outputlog.txt " %i)
    # os.system("go  test -race -run TestFailNoAgree2B  >> outputlog2.txt ")
    os.system("go  test -race -run 2B  >> outputlog.txt ")
    # os.system("go  test -race -run TestBasicAgree2B  >> outputlog.txt ")
    os.system("echo \'========= %d over =========\n\' >> outputlog.txt " %i)