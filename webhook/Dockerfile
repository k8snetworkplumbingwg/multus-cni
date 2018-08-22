FROM golang:1.10 as builder

RUN go get github.com/Masterminds/glide

ENV PKG_NAME=github.com/intel/multus-cni/webhook
ENV PKG_PATH=$GOPATH/src/$PKG_NAME
WORKDIR $PKG_PATH

COPY *.go glide.yaml $PKG_PATH/
RUN glide install -v
RUN go build -v -o webhook

FROM golang:1.10
ENV PKG_NAME=github.com/intel/multus-cni/webhook
ENV PKG_PATH=$GOPATH/src/$PKG_NAME
RUN mkdir /webhook
COPY --from=builder $PKG_PATH/webhook /webhook/
WORKDIR /webhook
CMD ["./webhook"]
