module code.cloudfoundry.org/silk

go 1.20

replace github.com/containernetworking/plugins => github.com/containernetworking/plugins v0.6.1-0.20171122160932-92c634042c38

replace github.com/containernetworking/cni => github.com/containernetworking/cni v0.6.0

require (
	code.cloudfoundry.org/cf-networking-helpers v0.0.0-20230328193716-3c4df5fd5663
	code.cloudfoundry.org/debugserver v0.0.0-20230328160250-c4f3fe4b289a
	code.cloudfoundry.org/filelock v0.0.0-20230302172038-1783f8b1c987
	code.cloudfoundry.org/lager/v3 v3.0.1
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.0.0-00010101000000-000000000000
	github.com/go-sql-driver/mysql v1.7.0
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/jmoiron/sqlx v1.3.5
	github.com/lib/pq v1.10.7
	github.com/onsi/ginkgo/v2 v2.9.2
	github.com/onsi/gomega v1.27.4
	github.com/pivotal-cf-experimental/gomegamatchers v0.0.0-20180326192815-e36bfcc98c3a
	github.com/rubenv/sql-migrate v1.4.0
	github.com/tedsuo/ifrit v0.0.0-20220120221754-dd274de71113
	github.com/tedsuo/rata v1.0.0
	github.com/vishvananda/netlink v1.1.0
	github.com/ziutek/utils v0.0.0-20190626152656-eb2a3b364d6c
	gopkg.in/validator.v2 v2.0.1
)

require (
	code.cloudfoundry.org/bbs v0.0.0-20210727125654-2ad50317f7ed // indirect
	github.com/alexflint/go-filemutex v1.1.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cloudfoundry/gosteno v0.0.0-20150423193413-0c8581caea35 // indirect
	github.com/cloudfoundry/loggregatorlib v0.0.0-20170823162133-36eddf15ef12 // indirect
	github.com/cloudfoundry/sonde-go v0.0.0-20200416163440-a42463ba266b // indirect
	github.com/coreos/go-iptables v0.6.0 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/pprof v0.0.0-20230323073829-e72429f035bd // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/openzipkin/zipkin-go v0.4.1 // indirect
	github.com/square/certstrap v1.2.0 // indirect
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.7.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 // indirect
)
