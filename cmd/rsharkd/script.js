var url =  window.location.protocol + "//" + window.location.host
var applyurl = url + "/config/apply"
var geturl = url + "/config/get"
var streamurl = window.location.protocol + "//" + window.location.hostname + ":8000/status-json.xsl"

function handleResponse(fn) {
	return function(ev) {
	    if (this.status != 200) {
            try {
                var obj = JSON.parse(this.responseText)
                setStatusText(obj["error"], "#ff0000")
            } catch(exp) {
                setStatusText(this.responseText, "#ff0000")
            }
        } else {
            if (fn != null) {
                fn(this)
            }
        }
    }
}

function updateForm(resp) {
    var obj = JSON.parse(resp.responseText)
    if (obj["modulation"] == "AM" || obj["modulation"] == "am") {
        document.getElementById("AM").checked = true
    } else if (obj["modulation"] == "FM" || obj["modulation"] == "fm") {
        document.getElementById("FM").checked = true
    }
    document.getElementById("frequency").value = obj["frequency"]
    setStatusText("OK", "#00ff00")
}

function setStatusText(text, color) {
    var status = document.getElementById("status")
    status.innerHTML = text
    status.style.color = color
}

document.addEventListener('DOMContentLoaded', function(){
    var req = new XMLHttpRequest()
    req.addEventListener("load", handleResponse(updateForm))
    req.addEventListener("error", handleResponse())
    req.open("GET", geturl)
    req.send()
    var streamReq =  new XMLHttpRequest()
    streamReq.addEventListener("load", handleResponse(function(resp) {
        var obj = JSON.parse(resp.responseText)
        var listenurl = obj["icestats"]["source"]["listenurl"]
        var parent = document.getElementById("stream")
        var listenLink = document.createElement("a")
        listenLink.href = listenurl
        listenLink.innerHTML = "Listen"
        var m3uLink = document.createElement("a")
        m3uLink.href = listenurl + ".m3u"
        m3uLink.innerHTML = "m3u"
        parent.appendChild(listenLink)
        parent.appendChild(document.createElement("br"))
        parent.appendChild(m3uLink)
    }))
    streamReq.addEventListener("error", handleResponse())
    streamReq.open("GET", streamurl)
    streamReq.send()
});

function apply() {
    var FD = new FormData()
    if (document.getElementById("AM").checked) {
        FD.append("modulation", "AM")
    } else if (document.getElementById("FM").checked) {
        FD.append("modulation", "FM")
    }
    FD.append("frequency", document.getElementById("frequency").value)
    var req = new XMLHttpRequest()
    req.open("POST", applyurl)
    req.send(FD)
    req.addEventListener("error", handleResponse())
    req.addEventListener("load", handleResponse(function(ev){
        setStatusText("OK", "#00ff00")
    }))
}
