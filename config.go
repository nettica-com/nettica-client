package main

//
// The /etc/nettica directory holds the configuration files for the Nettica Client and Agent.  Each file contains a JSON message object with each server's specific configuration.
// The files are named using the server name and the .json extension.  Service host configuration files are in server.name-service-host.json file.  A Message object looks like this:
//
//

/*
{
	"version" : "3.0",
    "id": "device-Ln7qwN2dA27G",
    "device": {
        "id": "device-Ln7qwN2dA27G",
        "server": "https://my.nettica.com",
        "apiKey": "device-api-...",
        "accountid": "account-aa5F...",
        "name": "gabba1",
        "description": "",
        "enable": true,
        "tags": null,
        "platform": "Windows",
        "os": "windows",
        "arch": "amd64",
        "clientid": "FuE6p0feioTrAypLhkcMhoskpsVs8YfK",		// remove
        "authdomain": "auth.nettica.com",					// remove
        "apiid": "http://meshify-resource-server",			// remove
        "appdata": "undefined",								// remove
        "checkInterval": 10,
        "sourceAddress": "0.0.0.0",
        "registered": true,
        "updateKeys": false,
        "version": "3.0",
        "createdBy": "alan@nettica.com",
        "updatedBy": "alan1",
        "created": "2023-10-20T00:18:24.959598155Z",
        "updated": "2024-05-30T02:47:37.273660218Z"
    },
    "config": [
        {
            "netName": "africa",
            "netid": "net-WMboKtEwOvhG",
            "vpns": [
                {
                    "id": "vpn-T3pAdHzY4InI",
                    "accountid": "account-Va5F1mfmQ1OcBrif",
                    "deviceid": "device-Ln7qwN2dA27G",
                    "name": "alan1.africa",
                    "netid": "net-WMboKtEwOvhG",
                    "netName": "africa",
                    "failover": 0,
                    "failCount": 0,
                    "enable": false,
                    "tags": [],
                    "createdBy": "alan@nettica.com",
                    "updatedBy": "alan1",
                    "created": "2024-02-02T12:19:04.221032726Z",
                    "updated": "2024-05-25T01:47:48.326262206Z",
                    "current": {
                        "privateKey": "",
                        "publicKey": "0Ch/wVQK4olThS5/uh/G//JYHdrjdP8AkIZho32VPQ0=",
                        "presharedKey": "LhJNrRHV5vyhHh/mTCmz1ZqMqFEDOPjpiWg6YOptrNI=",
                        "allowedIPs": [
                            "10.10.25.2/32"
                        ],
                        "address": [
                            "10.10.25.2/32"
                        ],
                        "dns": [
                            "1.1.1.1"
                        ],
                        "persistentKeepalive": 0,
                        "listenPort": 0,
                        "endpoint": "",
                        "mtu": 0,
                        "postDownomitempty": ""
                    },
                    "default": {
                        "privateKey": "",
                        "publicKey": "",
                        "presharedKey": "LhJNrRHV5vyhHh/mTCmz1ZqMqFEDOPjpiWg6YOptrNI=",
                        "allowedIPs": [
                            "10.10.25.0/24"
                        ],
                        "address": [
                            "10.10.25.0/24"
                        ],
                        "dns": [
                            "1.1.1.1"
                        ],
                        "persistentKeepalive": 0,
                        "listenPort": 0,
                        "endpoint": "",
                        "mtu": 0,
                        "postDownomitempty": ""
                    }
                },
            ]
        }
    ]
}
*/
