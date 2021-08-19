module code.cloudfoundry.org/silk

go 1.16

replace code.cloudfoundry.org/cf-networking-helpers => code.cloudfoundry.org/cf-networking-helpers v0.0.0-20210818160341-4ed163a496a8

replace github.com/containernetworking/plugins => github.com/containernetworking/plugins v0.6.1-0.20171122160932-92c634042c38

replace github.com/square/certstrap => github.com/square/certstrap v1.1.1

require (
	code.cloudfoundry.org/bbs v0.0.0-20210727125654-2ad50317f7ed // indirect
	code.cloudfoundry.org/cf-networking-helpers v0.0.0-20210809162311-5c15f1e28181
	code.cloudfoundry.org/debugserver v0.0.0-20210608171006-d7658ce493f4
	code.cloudfoundry.org/filelock v0.0.0-20180314203404-13cd41364639
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/alexflint/go-filemutex v1.1.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/cloudfoundry/gosteno v0.0.0-20150423193413-0c8581caea35 // indirect
	github.com/cloudfoundry/sonde-go v0.0.0-20200416163440-a42463ba266b // indirect
	github.com/containernetworking/cni v0.6.0
	github.com/containernetworking/plugins v0.0.0-00010101000000-000000000000
	github.com/coreos/go-iptables v0.6.0 // indirect
	github.com/go-sql-driver/mysql v1.6.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/jmoiron/sqlx v1.3.4
	github.com/lib/pq v1.10.2
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/pivotal-cf-experimental/gomegamatchers v0.0.0-20180326192815-e36bfcc98c3a
	github.com/rubenv/sql-migrate v0.0.0-20210614095031-55d5740dbbcc
	github.com/square/certstrap v1.2.0 // indirect
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	github.com/tedsuo/rata v1.0.0
	github.com/vishvananda/netlink v1.1.0
	github.com/ziutek/utils v0.0.0-20190626152656-eb2a3b364d6c
	gopkg.in/validator.v2 v2.0.0-20210331031555-b37d688a7fb0
)
