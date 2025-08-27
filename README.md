# Psyzkaller: Improving Linux Kernel Fuzzing with Syscall Dependency Relationship Learning


## Documentation

Psyzkaller is an advanced kernel fuzzing framework that extends Google's Syzkaller with innovative Syscall Dependency Relationship (SDR) learning capabilities. By leveraging N-gram models and RandomWalk sequence generation, Psyzkaller significantly enhances test case quality, enabling deeper kernel exploration and more effective vulnerability discovery.

###  Key Innovations

- Advanced SDR Learning: Utilizes N-gram statistical models to learn complex syscall dependencies from both the DongTing dataset and Syzkaller-generated corpus

- Intelligent Test Generation: Implements RandomWalk algorithm to generate syscall sequences that respect learned dependency constraints

- Comprehensive Coverage: Achieves 4.6%-7.0% higher code coverage across Linux kernel versions 5.19, 6.1, and 6.12

- Superior Vulnerability Detection: Identifies 110.4%-187.2% more kernel crashes and discovered 8 previously unknown vulnerabilities (vs Syzkaller's 1)

## Quick Start
We have successfully deployed Psyzkaller on Ubuntu 22.04 and 20.04. The setup process is straightforward:
```
# Install Go toolchain (required for building Psyzkaller)
wget https://dl.google.com/go/go1.21.4.linux-amd64.tar.gz
tar -xf go1.21.4.linux-amd64.tar.gz
export GOROOT=`pwd`/go
export PATH=$GOROOT/bin:$PATH

# Clone and build Psyzkaller
git clone https://github.com/anonimiPsyz/Psyzkaller.git
cd Psyzkaller
make
```
For complete environment setup instructions, including kernel compilation and disk image creation, please refer to the detailed guide in [setup.md](https://github.com/anonimiPsyz/Psyzkaller/blob/anonimize/docs/linux/setup.md).
Psyzkaller maintains full compatibility with standard syzkaller configuration files. See this [example configuration](https://github.com/anonimiPsyz/Psyzkaller/blob/anonimize/psyz-scripts-and-data/psyzkaller.cfg.sample) for reference.
To run Psyzkaller with N-gram mode:
``` 
./bin/syz-manager -psyzMode=NTD -DTJson=nor1vabn1.json -config=psyzkaller.cfg 
```
Pre-trained N-gram models and configuration templates are available in the [psyz-scripts-and-data](https://github.com/anonimiPsyz/Psyzkaller/tree/anonimize/psyz-scripts-and-data) directory.

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