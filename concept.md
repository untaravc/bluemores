- Create golang fiber app, this will be app like node exporter and prometheus. (server monitoring). this app will install in several server as exporter, also instaled in a server to show the graphic / result. use sqllite to save the data result
- has route get, will receive header bearer to validate key
    - /mon/proc -> will return the current processor condition like
        {
            "status": true,
            "data": {
                "value": 120
                "total": 400
            }
        }
    - /mon/mem -> will return the current processor condition like
        {
            "status": true,
            "data": {
                "unit": "MB",
                "value": 1200,
                "total": 4096
            }
        }
    - /mon/dfree -> will return the current processor condition like
        {
            "status": true,
            "data": {
                "unit": "GB",
                "value": 120,
                "total": 150
            }
        }

- has scheduler to hit several other website

- has config in json to config where to hit, how often hit like
    [
        {
            "group": "Server Tries",
            "apps": [
                {
                    "name": "Processor"
                    "type": "processor"
                    "source": "https://mon.tries.co.id/mon/proc",
                    "schedule": "* * * * *",
                    "threshold": "300",
                    "notification": "https://discord.com/xxx",
                },
                {
                    "name": "Memory"
                    "type": "memory"
                    "source": "https://mon.tries.co.id/mon/mem",
                    "schedule": "*/5 * * * *",
                    "threshold": "3000",
                    "notification": "https://discord.com/xxx",
                },
                {
                    "name": "Disk Free"
                    "type": "disk_free"
                    "source": "https://mon.tries.co.id/mon/dfree",
                    "schedule": "0 0 * * *",
                    "threshold": "100",
                    "notification": "https://discord.com/xxx",
                },
                {
                    "name": "BE Health"
                    "type": "health"
                    "source": "https://tries.co.id/health",
                    "schedule": "0 0 * * *",
                    "threshold": "true",
                    "notification": "https://discord.com/xxx",
                },
            ],
        }
    ]

- has page to show the result based on the configs. to open the page, should enter passprase saved in .env

- we will has like sqlite to save the result of monitoring