var br = chrome;
var omnom_url = '';
var omnom_token = '';
var site_url = '';
var is_ff = typeof InstallTrigger !== 'undefined';
var is_chrome = !is_ff;
var debug = false;

function debugPopup(content) {
    if(is_chrome) {
        var win = window.open("", "omnomDebug", "menubar=yes,location=yes,resizable=yes,scrollbars=yes,status=yes");
        win.document.write(content);
    } else {
        document.getElementsByTagName('html')[0].innerHTML = content;
    }
    console.log(content);
}

function saveBookmark(e) {
	e.preventDefault();
    createSnapshot().then(async (result) => {
        if(debug) {
            debugPopup(result);
            return;
        }
        var form = new FormData(document.forms['add']);
        form.append("snapshot", result);
        fetch(omnom_url+'add_bookmark', {
            method: 'POST',
            body: form,
            //headers: {
            //    'Content-type': 'application/json; charset=UTF-8'
            //}
        })
        .then(resp => {
            if(resp.status !== 200) {
                throw Error(resp.statusText);
            }
            document.body.innerHTML = '<h1>Bookmark saved</h1>';
            setTimeout(window.close, 2000);
        })
        .catch(err => {
            document.body.innerHTML = '<h1>Failed to save bookmark: '+err+'</h1>';
        });
    });
}

function displayPopup() {
    document.querySelector("form").addEventListener("submit", saveBookmark);
    br.storage.local.get(['omnom_url', 'omnom_token', 'omnom_debug'], function(data) {
        omnom_url = data.omnom_url || '';
        omnom_token = data.omnom_token || '';
        debug = data.omnom_debug || false;
        document.getElementById("omnom_options").addEventListener("click", function() {
            br.runtime.openOptionsPage(function() {
                window.close();
            });
        });
        if(omnom_url == '') {
            document.getElementById("omnom_content").innerHTML = '<h1>Server URL not found. Specify it in the extension\'s options</h1>';
            return;
        }
        if(omnom_token == '') {
            document.getElementById("omnom_content").innerHTML = '<h1>Token not found. Specify it in the extension\'s options</h1>';
            return;
        }
        document.getElementById("omnom_url").innerHTML = "Server URL: "+omnom_url;
        document.querySelector("form").action = omnom_url+'add_bookmark';
        document.getElementById("token").value = omnom_token;
        // fill url input field
        var url;
        br.tabs.query({active: true, lastFocusedWindow: true}, (tabs) => {
            document.getElementById('url').value = tabs[0].url;
            site_url = tabs[0].url;
        });
        // fill title input field
        br.tabs.executeScript({
            code: 'document.title;'
        }, (title) => {
            if(title && title[0]) {
                document.getElementById('title').value = title[0];
            }
        });
        // fill notes input field
        br.tabs.executeScript( {
            code: "window.getSelection().toString();"
        }, function(selection) {
            if(selection && selection[0]) {
                document.getElementById("notes").value = selection[0];
            }
        });
    });
}

function rewriteAttributes(node) {
    for(var i=0; i <  node.attributes.length; i++) {
        var attr = node.attributes[i];
        if(attr.nodeName === undefined) {
            continue;
        }
        if(attr.nodeName.startsWith("on")) {
            attr.nodeValue = '';
        } else if(attr.nodeName.startsWith("data-")) {
            attr.nodeValue = '';
        } else if(attr.nodeValue.trim().toLowerCase().startsWith("javascript:")) {
            attr.nodeValue = '';
        }
        if(attr.nodeName == "href") {
            attr.nodeValue = fullURL(attr.nodeValue);
        }
    }
}

function getDOMData() {
    function fullURL(url) {
        return new URL(url, window.location.origin).href
    }
    function getCSSText(obj) {
        if(obj.cssText) {
            return obj.cssText;
        }
        var text = '';
        for(var i=0; i < obj.length; i++) {
            var key = obj.item(i);
            text += key + ':' + obj[key] + ';';
        }
        return text;
    }
    function walkDOM(node, func) {
        func(node);
        var children = node.childNodes;
        for (var i = 0; i < children.length; i++) {
            walkDOM(children[i], func);
        }
    }
    var html = document.getElementsByTagName('html')[0];
    var ret = {
        'html': html.cloneNode(true),
        'title': '',
        'doctype': '',
    };
    if(document.doctype) {
        ret.doctype = new XMLSerializer().serializeToString(document.doctype);
    }
    if(document.getElementsByTagName('title').length > 0) {
        ret.title = document.getElementsByTagName('title')[0].innerText;
    }
    var nodesToRemove = [];
    var hiddenStyleKeys = [
        'width',
        'height',
        'margin',
        'padding',
        'minWidth',
        'maxWidth',
        'minHeight',
        'maxHeight',
    ];
    walkDOM(html, async function(n) {
        if(n.nodeName == 'SCRIPT') {
            nodesToRemove.push(n);
            return;
        }
    });
    for(i in nodesToRemove) {
        nodesToRemove[i].remove();
    }
    ret.html = ret.html.outerHTML;
    return ret;
}

async function createSnapshot() {
    var doc;
    function getDOM() {
        return new Promise((resolve, error) => {
            br.tabs.executeScript({
                code: '('+getDOMData+')()'
            }, (data) => {
                if(data && data[0]) {
                    doc = data[0];
                    resolve('');
                } else {
                    error('meh');
                }
            });
        });
    }
    await getDOM();
    var dom = document.createElement('html');
    dom.innerHTML = doc.html;
    await walkDOM(dom, async function(node) {
        if(node.nodeType !== Node.ELEMENT_NODE) {
            return;
        }
        if(node.nodeName == 'SCRIPT') {
            node.remove();
            return;
        }
        await rewriteAttributes(node);
        if(node.nodeName == 'LINK' && node.attributes.type && node.attributes.type.nodeValue.trim().toLowerCase() == "text/css") {
            var cssHref = node.attributes.href.nodeValue;
            var style = document.createElement('style');
            var cssText = await inlineFile(cssHref);
            style.innerHTML = await sanitizeCSS(cssText);
            node.parentNode.appendChild(style);
            node.remove();
            return;
        }
        if(node.nodeName == 'IMG') {
            node.src = await inlineFile(node.getAttribute("src"));
        }
    });
    return doc.doctype+dom.outerHTML;
}

async function walkDOM(node, func) {
    await func(node);
    var children = node.childNodes;
    for (var i = 0; i < children.length; i++) {
        await walkDOM(children[i], func);
    }
}

async function rewriteAttributes(node) {
    for(var i=0; i <  node.attributes.length; i++) {
        var attr = node.attributes[i];
        if(attr.nodeName === undefined) {
            continue;
        }
        if(attr.nodeName.startsWith("on")) {
            attr.nodeValue = '';
        }
        if(attr.nodeValue.startsWith("javascript:")) {
            attr.nodeValue = '';
        }
        if(attr.nodeName == "href") {
            attr.nodeValue = fullURL(attr.nodeValue);
        }
        if(attr.nodeName == "style") {
            var sanitizedValue = await sanitizeCSS('a{'+attr.nodeValue+'}');
            attr.nodeValue = sanitizedValue.substr(4, sanitizedValue.length-6);
        }
    }
}

async function inlineFile(url) {
    if(!url || (url || '').startsWith('data:')) {
        return url;
    }
    url = fullURL(url);
    console.log("fetching "+url);
	var options = {
	  method: 'GET',
	  cache: 'default',
	};
    if(debug) {
        options.cache = 'no-cache';
    }
	var request = new Request(url, options);

	var obj = await fetch(request, options).then(async function(response) {
        var contentType = response.headers.get("Content-Type");
        if(contentType.toLowerCase().search("text") != -1) {
            // TODO use charset of the response
            var body = await response.text();
            return body;
        }
	    var buff = await response.arrayBuffer()
        var base64Flag = 'data:'+contentType+';base64,';
        return base64Flag + arrayBufferToBase64(buff);
	});
    return obj;
}

function arrayBufferToBase64(buffer) {
  var binary = '';
  var bytes = [].slice.call(new Uint8Array(buffer));
  bytes.forEach((b) => binary += String.fromCharCode(b));

  return btoa(binary);
}

function fullURL(url) {
    return new URL(url, site_url).href
}

function parseCSS(styleContent) {
    var doc = document.implementation.createHTMLDocument("");
    var styleElement = document.createElement("style");

    styleElement.textContent = styleContent;
    // the style will only be parsed once it is added to a document
    doc.body.appendChild(styleElement);
    return styleElement.sheet.cssRules;
}

async function sanitizeCSS(cssText) {
    var rules = parseCSS(cssText);
    var sanitizedCSS = '';
    for(var k in rules) {
        var v = rules[k];
        if(!v || !v.style) {
            continue;
        }
        var bgi = v.style.backgroundImage;
        if(bgi && bgi.startsWith('url("') && bgi.endsWith('")')) {
            var bgURL = bgi.substring(5, bgi.length-2);
            inlineImg = await inlineFile(bgURL);
            v.style.backgroundImage = 'url("'+inlineImg+'")';
        }
        sanitizedCSS += v.cssText;
    }
    return sanitizedCSS
}

document.addEventListener('DOMContentLoaded', displayPopup);

/* ---------------------------------*
 * End of omnom code                *
 * ---------------------------------*/
