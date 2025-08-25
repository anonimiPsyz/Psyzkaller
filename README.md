# Psyzkaller: Improving Linux Kernel Fuzzing with Syscall Dependency Relationship Learning


## Documentation

Psyzkaller is an advanced kernel fuzzing framework that extends Google's Syzkaller with innovative Syscall Dependency Relationship (SDR) learning capabilities. By leveraging N-gram models and RandomWalk sequence generation, Psyzkaller significantly enhances test case quality, enabling deeper kernel exploration and more effective vulnerability discovery.

###  Key Innovations

- Advanced SDR Learning: Utilizes N-gram statistical models to learn complex syscall dependencies from both the DongTing dataset and Syzkaller-generated corpus

- Intelligent Test Generation: Implements RandomWalk algorithm to generate syscall sequences that respect learned dependency constraints

- Comprehensive Coverage: Achieves 4.6%-7.0% higher code coverage across Linux kernel versions 5.19, 6.1, and 6.12

- Superior Vulnerability Detection: Identifies 110.4%-187.2% more kernel crashes and discovered 8 previously unknown vulnerabilities (vs Syzkaller's 1)

## Quick Start
```
# Clone the repository
git clone https://github.com/anonimiPsyz/Psyzkaller.git

# Build Psyzkaller
cd Psyzkaller
make

# Configure and run (see config examples in config/ directory)
./bin/syz-manager -psyzMode=NTD -DTJson=successor_Prope.json -config=my.cfg 
```

Psyzkaller uses the same configuration file as syzkaller.  [Example](https://github.com/google/syzkaller/blob/master/pkg/mgrconfig/testdata/qemu-example.cfg)

```
-psyzMode string
    psyzkaller's mode flag :
        N: enable psyzkaller's N-gram optimization.
        R: enable psyzkaller's random Walk optimization.
        D: enable psyzkaller's DongTing optimization. Need successor_Prope.json in workdir.
        e.g. psyzMode=DN  : means enable DongTing and N-gram optimizations. 
        default: no optimization.
-DTJson string
    DongTing pre_analysis result file name. Default successor_Prope.json
```