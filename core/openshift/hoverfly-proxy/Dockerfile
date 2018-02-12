############################################################
# Dockerfile to create hoverfly-proxy
# Hoverfly start in capture mode

FROM registry.access.redhat.com/rhel7

# File Author / Maintainer
LABEL author='jose.luis.aparicio.frances.sa@everis.com'

# target folder must include hoverctl and hoverfly 
COPY target/* /go/bin/

# Change GROUP ownership of go directory to group 0. 
# Also, give the permissions the owner has over this directory, to all users in the same group

RUN chown -R 0 /go && \ 
    chmod -R g=u /go

USER root

RUN mkdir /.hoverfly && \
    chmod 777  /.hoverfly


USER 1001

ENTRYPOINT ["/go/bin/hoverfly"]

CMD ["-capture"]

EXPOSE 8500 8888
