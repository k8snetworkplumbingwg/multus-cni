## Development Information

## How to build the multus-cni?

```
git clone https://github.com/intel/multus-cni.git
cd multus-cni
./build
```

## How to run CI tests?

Multus has go unit tests (based on ginkgo framework). Following commands drive CI tests manually in your environment:

```
sudo ./test.sh
```

## Logging Best Practices

Followings are multus logging best practices:

* Add `logging.Debugf()` at the begining of function
* In case of error handling, use `logging.Errorf()` with given error info
* `logging.Panicf()` only be used at very critical error (it should NOT used usually)


## CI Introduction

TBD
