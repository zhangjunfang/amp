FROM appcelerator/prometheus:1.7.1

COPY promctl.alpine /bin/promctl
COPY config/prometheus.yml  /etc/prometheus/prometheus.yml
COPY config/prometheus.tpl /etc/prometheus/prometheus.tpl

ENTRYPOINT [ "/bin/promctl" ]
CMD [ ]
