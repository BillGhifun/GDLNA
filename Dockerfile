#FROM alpine:latest
#FROM docker.wanpeng.top/library/alpine:latest
FROM docker.1ms.run/library/alpine:latest

#RUN apk add --no-cache build-base

#ENV CGO_ENABLED=0

#RUN apk add --no-cache gcc musl-dev

LABEL\
	org.opencontainers.image.authors="BillGhifun" \
	org.opencontainers.image.description="Ghifun GDLNA Sniffer Server" \
	org.opencontainers.image.title="GDLNA" \
	org.opencontainers.image.vendor="BillGhifun"

# 在容器根目录 创建一个 apps 目录
WORKDIR /gdlna

# 挂载容器目录
VOLUME ["/gdlna"]
VOLUME ["/gdlna/db"]
VOLUME ["/gdlna/www"]
VOLUME ["/gdlna/movies"]

# 拷贝当前目录下二进制可执行文件
COPY /GDLNA_arm_linux /gdlna/GDLNA
COPY /www/ /gdlna/www/

# 拷贝配置文件到容器中
COPY /Config.ini /gdlna/Config.ini

# 设置文件夹下所有文件的权限[含子目录]
RUN chmod -R 777 /gdlna/

# 设置编码
ENV LANG C.UTF-8

# 暴露端口
EXPOSE 1900/udp 8181/tcp

# 在容器中指定使用自定义网络和分配新的 IP 地址
ENTRYPOINT ["/gdlna/GDLNA"]
#CMD ["/s5p/Socks5Proxy"]