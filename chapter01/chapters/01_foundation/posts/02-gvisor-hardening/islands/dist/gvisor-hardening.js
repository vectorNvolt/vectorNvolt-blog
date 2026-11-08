(()=>{var dt={block:{viewH:450,outlines:[{x:20,y:20,w:920,h:410,label:"agent-host"},{x:40,y:50,w:670,h:260,label:"gVisor sandbox (runsc)",variant:"safe"}],nodes:{agent:{x:60,y:90,w:140,h:64,label:"Agent process",sub:["untrusted","root inside"],variant:"solid-danger"},sentry:{x:230,y:90,w:270,h:64,label:"Sentry",sub:["user-space kernel","systrap"],variant:"solid-safe"},overlay:{x:540,y:90,w:150,h:64,label:"Rootfs overlay",sub:["in host RAM","--overlay2=root:memory"]},gofer:{x:230,y:214,w:150,h:64,label:"Gofer",sub:["--directfs=false"]},netstack:{x:400,y:214,w:170,h:64,label:"Netstack",sub:["--network=sandbox","--net-raw=false"]},daemon:{x:740,y:214,w:180,h:64,label:"daemon.json",sub:["runtime registration","--allow-flag-override=false"]},kernel:{x:40,y:350,w:880,h:64,label:"Host kernel",sub:"reduced syscall surface",variant:"solid-dark"}},links:[{points:[[500,122],[540,122]],dir:"end"}],ips:[{id:"IP-1",label:"1",points:[[200,122],[230,122]],dir:"both",at:[215,122]},{id:"IP-2",label:"2",points:[[305,154],[305,214]],dir:"both",at:[305,184]},{id:"IP-3",label:"3",points:[[485,154],[485,214]],dir:"both",at:[485,184]},{id:"IP-4",label:"4",points:[[305,278],[305,350]],dir:"end",at:[305,330]},{id:"IP-5",label:"5",points:[[485,278],[485,350]],dir:"end",at:[485,330]},{id:"IP-6",label:"6",points:[[740,246],[710,246]],dir:"end",at:[725,246]},{id:"IP-7",label:"7",points:[[390,154],[390,350]],dir:"end",at:[390,184]}],assets:[{id:"AS-1",label:"1",at:[690,154]},{id:"AS-2",label:"2",at:[740,278]}],threats:[{id:"T1a",label:"1a",at:[500,90]},{id:"T1b",label:"1b",at:[540,154]}]},blockText:{label:"Block diagram of the gVisor runtime boundary: the agent process talks only to the Sentry, which reaches the host kernel through the Gofer and Netstack and through a narrow set of its own host syscalls; the rootfs overlay lives in host RAM and the runtime is registered in daemon.json with flag override disabled",hint:"Circles are interaction points, diamonds assets, squares threats; hover or click one to see what it connects to. The flags in each box are the settings this post applies.",items:{"IP-1":{name:"Syscalls from the agent process",desc:"The agent\u2019s syscalls are handled by the Sentry, a kernel implemented in user space, instead of going straight to the host kernel. With the systrap platform they are intercepted and passed to the Sentry; the exact mechanism depends on the platform in use.",code:"--platform=systrap",links:["IP-2","IP-3","IP-7","T1a"]},"IP-2":{name:"File access through the Gofer",desc:"The Sentry is intended to reach host files only through the Gofer, which brokers the access. Turning off directfs keeps that brokering in place rather than handing the Sentry direct host file descriptors.",code:"--directfs=false",links:["IP-4","AS-1"]},"IP-3":{name:"Network through Netstack",desc:"Network traffic is handled by Netstack, the network stack that runs inside the sandbox. Raw sockets are switched off, which is meant to narrow what a root process in the sandbox can craft.",code:"--network=sandbox --net-raw=false",links:["IP-5"]},"IP-4":{name:"Gofer to host kernel",desc:"The Gofer performs file operations on the host on the sandbox\u2019s behalf. These are ordinary host syscalls, but a smaller and more predictable set than an arbitrary container process could issue.",links:["IP-7","T1a"]},"IP-5":{name:"Netstack to host kernel",desc:"Packets leave the sandbox through a small set of host interfaces rather than through the agent\u2019s own socket calls.",links:["IP-7","T1a"]},"IP-7":{name:"Sentry\u2019s own host syscalls",desc:"The Sentry still calls the host kernel for its own operation (memory mapping, futex, timers, epoll, I/O on host file descriptors), through a narrow set of syscalls filtered by seccomp. With --directfs enabled it also uses host file descriptors directly; this post turns it off.",links:["IP-4","IP-5","T1a"]},"IP-6":{name:"Runtime registration",desc:"The runtime is registered with Docker in daemon.json, together with its flags. Setting --allow-flag-override=false is meant to stop a container from supplying its own runtime flags and loosening these settings.",code:"daemon.json::runtimes.runsc.runtimeArgs",links:["AS-2"]},"AS-1":{name:"Rootfs overlay in host RAM",desc:"With --overlay2=root:memory the container\u2019s root filesystem writes go to an overlay held in memory instead of the image layers on disk. Changes are expected to disappear when the container stops.",code:"--overlay2=root:memory",links:["T1b"]},"AS-2":{name:"daemon.json runtime entry",desc:"The file that tells Docker how to launch the sandboxed runtime. Anyone who can edit it could change which flags apply, so it is treated as part of the trusted host configuration.",code:"--allow-flag-override=false"},T1a:{name:"Sentry escape",desc:"A bug in the Sentry could let a process break out of the sandbox. This is the main residual risk of the design. The Sentry is intended to be a small, memory-safe component, but that reduces the likelihood rather than ruling it out."},T1b:{name:"Memory pressure from the overlay",desc:"Because the root overlay is held in host RAM, a process that writes a lot of data can consume memory. A memory limit on the container is the usual way to bound this; whether it is sufficient depends on the workload."}}}},E={viewH:410,host:{x:20,y:20,w:920,h:370,label:"Host"},kernel:{x:40,y:310,w:880,h:64}},lt={block:{viewH:E.viewH,outlines:[E.host,{x:40,y:60,w:200,h:130,label:"runc container",variant:"danger"}],nodes:{agent:{x:60,y:100,w:160,h:64,label:"Agent process",sub:["root inside","SYS_ADMIN granted"],variant:"solid-danger"},check:{x:720,y:100,w:200,h:64,label:"Capability check",sub:["host kernel, not seccomp","process capability sets"]},kernel:{...E.kernel,label:"Host kernel",sub:"shared with container",variant:"solid-danger"}},links:[],ips:[{id:"IP-R1",label:"R1",points:[[140,164],[140,310]],dir:"end",at:[140,250]},{id:"IP-R2",label:"R2",points:[[820,310],[820,164]],dir:"end",at:[820,250]}],assets:[],threats:[{id:"T-R1",label:"R1",at:[300,310]},{id:"T-R2",label:"R2",at:[900,310]}]},blockText:{label:"Block diagram of SYS_ADMIN under runc: the agent process\u2019s mount and namespace syscalls go directly to the host kernel, whose own capability check evaluates them against the container process\u2019s capability sets",hint:"Circles are interaction points, squares threats; hover or click one to see what it connects to.",items:{"IP-R1":{name:"Mount and namespace syscalls reach the host kernel",desc:"Under runc the container is a set of namespaces and cgroups on the host. Calls such as mount, unshare and setns are handled by the host kernel itself. Whether one succeeds depends on the capability check there, which is evaluated against the host\u2019s user namespace rules.",code:"mount(2), unshare(2), setns(2)",links:["IP-R2","T-R1","T-R2"]},"IP-R2":{name:"Host kernel checks the capability",desc:"The check is the host kernel\u2019s own, not seccomp and not a separate component. For each privileged call the kernel evaluates the capability against the container process\u2019s credentials and capability sets, which are bounded by the --cap-add and --cap-drop set Docker applied. If the capability is present, the operation runs in the host kernel.",code:"capable(CAP_SYS_ADMIN) in the host kernel",links:["T-R2"]},"T-R1":{name:"Kernel attack surface",desc:"The whole host syscall interface is reachable from the container. A flaw in any code path it can reach is a flaw in the kernel the host and every other container depend on."},"T-R2":{name:"Container escape is host access",desc:"Because the kernel is shared, an escape does not cross a further boundary: it lands in the host kernel\u2019s context. Granting SYS_ADMIN widens the set of operations an attacker can attempt."}}}},ct={block:{viewH:E.viewH,outlines:[E.host,{x:40,y:60,w:660,h:220,label:"gVisor sandbox (runsc)",variant:"safe"}],nodes:{agent:{x:60,y:100,w:160,h:64,label:"Agent process",sub:["root inside","SYS_ADMIN granted"],variant:"solid-danger"},sentry:{x:290,y:100,w:180,h:64,label:"Sentry",sub:["user-space kernel"],variant:"solid-safe"},io:{x:540,y:100,w:150,h:64,label:"Gofer \xB7 Netstack",sub:["brokered I/O"]},check:{x:340,y:204,w:180,h:64,label:"Capability check",sub:["inside Sentry, not seccomp","emulated credentials"]},kernel:{...E.kernel,label:"Host kernel",sub:"reduced syscall surface",variant:"solid-dark"}},links:[],ips:[{id:"IP-G1",label:"G1",points:[[220,132],[290,132]],dir:"end",at:[255,132]},{id:"IP-G2",label:"G2",points:[[470,132],[540,132]],dir:"both",at:[505,132]},{id:"IP-G3",label:"G3",points:[[615,164],[615,310]],dir:"end",at:[615,296]},{id:"IP-G4",label:"G4",points:[[380,164],[380,204]],dir:"end",at:[380,184]},{id:"IP-G5",label:"G5",points:[[310,164],[310,310]],dir:"end",at:[310,250]}],assets:[],threats:[{id:"T-G1",label:"G1",at:[470,100]},{id:"T-G2",label:"G2",at:[880,310]}]},blockText:{label:"Block diagram of SYS_ADMIN under gVisor: the agent process\u2019s syscalls go to the Sentry, which runs its own emulated capability check against the sandbox\u2019s emulated credentials for other syscalls; the host kernel is reached by the Gofer and Netstack and by a narrow seccomp-filtered set of the Sentry\u2019s own host syscalls, but not by the capability check",hint:"Circles are interaction points, squares threats; hover or click one to see what it connects to.",items:{"IP-G1":{name:"Syscalls arrive at the Sentry",desc:"The same mount, unshare and setns calls are answered by the Sentry. The capability check runs against the credentials the Sentry keeps for the sandbox, so SYS_ADMIN here is a property of the emulated kernel, not of the host.",code:"mount(2), unshare(2), setns(2)",links:["IP-G2","IP-G4","IP-G5","T-G1"]},"IP-G4":{name:"Other syscalls, handled inside the Sentry",desc:"Syscalls that are neither file nor network I/O, such as mount or a capability-gated call, are answered inside the Sentry. Its capability check is its own emulation, run against the sandbox\u2019s emulated credentials: it is not the host kernel and not seccomp, and these calls do not reach the host kernel.",code:"mount(2), unshare(2), setns(2)",links:["T-G1"]},"IP-G2":{name:"Sentry to Gofer and Netstack",desc:"File and network work is delegated to the Gofer and to Netstack, which are the components that touch host resources.",links:["IP-G3"]},"IP-G3":{name:"Remaining host syscalls",desc:"The host kernel is still reached, but through a restricted set of syscalls issued by the sandbox\u2019s own components rather than by the agent process. The Sentry makes some host syscalls of its own as well (see the Sentry\u2019s own host syscalls).",links:["IP-G5","T-G2"]},"IP-G5":{name:"Sentry\u2019s own host syscalls",desc:"The Sentry still calls the host kernel for its own operation (memory mapping, futex, timers, epoll, I/O on host file descriptors), through a narrow set of syscalls filtered by seccomp. With --directfs enabled it also uses host file descriptors directly. The agent\u2019s syscalls that the Sentry answers itself, including the capability check, do not appear here.",links:["IP-G3","T-G2"]},"T-G1":{name:"Sentry bug",desc:"A flaw in the Sentry\u2019s implementation of a syscall could let a process act beyond its emulated privileges. The boundary moves from the host kernel to a smaller, separate component; it does not disappear."},"T-G2":{name:"Residual host exposure",desc:"The host kernel is still part of the path. The aim is a smaller reachable surface than under runc, not zero exposure."}}}},te={runtime:dt,runc:lt,gvisor:ct};var V,b,ye,gt,R,be,xe,we,ne,L,M,Se,se,ae,re,Pe,U={},q=[],ht=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,Y=Array.isArray;function I(t,e){for(var n in e)t[n]=e[n];return t}function ie(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function ut(t,e,n){var a,o,r,i={};for(r in e)r=="key"?a=e[r]:r=="ref"?o=e[r]:i[r]=e[r];if(arguments.length>2&&(i.children=arguments.length>3?V.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(r in t.defaultProps)i[r]===void 0&&(i[r]=t.defaultProps[r]);return z(t,i,a,o,null)}function z(t,e,n,a,o){var r={type:t,props:e,key:n,ref:a,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:o==null?++ye:o,__i:-1,__u:0};return o==null&&b.vnode!=null&&b.vnode(r),r}function P(t){return t.children}function F(t,e){this.props=t,this.context=e}function $(t,e){if(e==null)return t.__?$(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?$(t):null}function ft(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,a=[],o=[],r=I({},e);r.__v=e.__v+1,b.vnode&&b.vnode(r),de(t.__P,r,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,a,n==null?$(e):n,!!(32&e.__u),o),r.__v=e.__v,r.__.__k[r.__i]=r,Ce(a,r,o),e.__e=e.__=null,r.__e!=n&&Te(r)}}function Te(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),Te(t)}function oe(t){(!t.__d&&(t.__d=!0)&&R.push(t)&&!j.__r++||be!=b.debounceRendering)&&((be=b.debounceRendering)||xe)(j)}function j(){try{for(var t,e=1;R.length;)R.length>e&&R.sort(we),t=R.shift(),e=R.length,ft(t)}finally{R.length=j.__r=0}}function Ie(t,e,n,a,o,r,i,l,g,c,f){var p,s,u,v,k,d,_=a&&a.__k||q,m=e.length;for(g=pt(n,e,_,g,m),p=0;p<m;p++)(u=n.__k[p])!=null&&(s=u.__i!=-1&&_[u.__i]||U,u.__i=p,d=de(t,u,s,o,r,i,l,g,c,f),v=u.__e,u.ref&&s.ref!=u.ref&&(s.ref&&le(s.ref,null,u),f.push(u.ref,u.__c||v,u)),k==null&&v!=null&&(k=v),4&u.__u?(g=Ae(u,g,t),s.__e&&(s.__e=null)):typeof u.type=="function"&&d!==void 0?g=d:v&&(g=v.nextSibling),u.__u&=-7);return n.__e=k,g}function pt(t,e,n,a,o){var r,i,l,g,c,f=n.length,p=f,s=0;for(t.__k=new Array(o),r=0;r<o;r++)(i=e[r])!=null&&typeof i!="boolean"&&typeof i!="function"?(typeof i=="string"||typeof i=="number"||typeof i=="bigint"||i.constructor==String?i=t.__k[r]=z(null,i,null,null,null):Y(i)?i=t.__k[r]=z(P,{children:i},null,null,null):i.constructor===void 0&&i.__b>0?i=t.__k[r]=z(i.type,i.props,i.key,i.ref?i.ref:null,i.__v):t.__k[r]=i,g=r+s,i.__=t,i.__b=t.__b+1,l=null,(c=i.__i=_t(i,n,g,p))!=-1&&(p--,(l=n[c])&&(l.__u|=2)),l==null||l.__v==null?(c==-1&&(o>f?s--:o<f&&s++),typeof i.type!="function"&&(i.__u|=4)):c!=g&&(c==g-1?s--:c==g+1?s++:(c>g?s--:s++,i.__u|=4))):t.__k[r]=null;if(p)for(r=0;r<f;r++)(l=n[r])!=null&&!(2&l.__u)&&(l.__e==a&&(a=$(l)),De(l,l));return a}function Ae(t,e,n){var a,o;if(typeof t.type=="function"){for(a=t.__k,o=0;a&&o<a.length;o++)a[o]&&(a[o].__=t,e=Ae(a[o],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=$(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function _t(t,e,n,a){var o,r,i,l=t.key,g=t.type,c=e[n],f=c!=null&&(2&c.__u)==0;if(c===null&&l==null||f&&l==c.key&&g==c.type)return n;if(a>(f?1:0)){for(o=n-1,r=n+1;o>=0||r<e.length;)if((c=e[i=o>=0?o--:r++])!=null&&!(2&c.__u)&&l==c.key&&g==c.type)return i}return-1}function ve(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||ht.test(e)?n:n+"px"}function B(t,e,n,a,o){var r,i;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof a=="string"&&(t.style.cssText=a=""),a)for(e in a)n&&e in n||ve(t.style,e,"");if(n)for(e in n)a&&n[e]==a[e]||ve(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")r=e!=(e=e.replace(Se,"$1")),i=e.toLowerCase(),e=i in t||e=="onFocusOut"||e=="onFocusIn"?i.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+r]=n,n?a?n[M]=a[M]:(n[M]=se,t.addEventListener(e,r?re:ae,r)):t.removeEventListener(e,r?re:ae,r);else{if(o=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function ke(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[L]==null)e[L]=se++;else if(e[L]<n[M])return;return n(b.event?b.event(e):e)}}}function de(t,e,n,a,o,r,i,l,g,c){var f,p,s,u,v,k,d,_,m,S,A,C,H,me,W,ee,T=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(g=!!(32&n.__u),r=[l=e.__e=n.__e]),(f=b.__b)&&f(e);e:if(typeof T=="function"){p=i.length;try{if(m=e.props,S=T.prototype&&T.prototype.render,A=(f=T.contextType)&&a[f.__c],C=f?A?A.props.value:f.__:a,n.__c?_=(s=e.__c=n.__c).__=s.__E:(S?e.__c=s=new T(m,C):(e.__c=s=new F(m,C),s.constructor=T,s.render=bt),A&&A.sub(s),s.state||(s.state={}),s.__n=a,u=s.__d=!0,s.__h=[],s._sb=[]),S&&s.__s==null&&(s.__s=s.state),S&&T.getDerivedStateFromProps!=null&&(s.__s==s.state&&(s.__s=I({},s.__s)),I(s.__s,T.getDerivedStateFromProps(m,s.__s))),v=s.props,k=s.state,s.__v=e,u)S&&T.getDerivedStateFromProps==null&&s.componentWillMount!=null&&s.componentWillMount(),S&&s.componentDidMount!=null&&s.__h.push(s.componentDidMount);else{if(S&&T.getDerivedStateFromProps==null&&m!==v&&s.componentWillReceiveProps!=null&&s.componentWillReceiveProps(m,C),e.__v==n.__v||!s.__e&&s.shouldComponentUpdate!=null&&s.shouldComponentUpdate(m,s.__s,C)===!1){e.__v!=n.__v&&(s.props=m,s.state=s.__s,s.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(D){D&&(D.__=e)}),q.push.apply(s.__h,s._sb),s._sb=[],s.__h.length&&i.push(s),l=$(n);break e}s.componentWillUpdate!=null&&s.componentWillUpdate(m,s.__s,C),S&&s.componentDidUpdate!=null&&s.__h.push(function(){s.componentDidUpdate(v,k,d)})}if(s.context=C,s.props=m,s.__P=t,s.__e=!1,H=b.__r,me=0,S)s.state=s.__s,s.__d=!1,H&&H(e),f=s.render(s.props,s.state,s.context),q.push.apply(s.__h,s._sb),s._sb=[];else do s.__d=!1,H&&H(e),f=s.render(s.props,s.state,s.context),s.state=s.__s;while(s.__d&&++me<25);s.state=s.__s,s.getChildContext!=null&&(a=I(I({},a),s.getChildContext())),S&&!u&&s.getSnapshotBeforeUpdate!=null&&(d=s.getSnapshotBeforeUpdate(v,k)),W=f!=null&&f.type===P&&f.key==null?$e(f.props.children):f,l=Ie(t,Y(W)?W:[W],e,n,a,o,r,i,l,g,c),s.base=e.__e,e.__u&=-161,s.__h.length&&i.push(s),_&&(s.__E=s.__=null)}catch(D){if(i.length=p,e.__v=null,g||r!=null){if(D.then){for(e.__u|=g?160:128;l&&l.nodeType==8&&l.nextSibling;)l=l.nextSibling;r!=null&&(r[r.indexOf(l)]=null),e.__e=l}else if(r!=null)for(ee=r.length;ee--;)ie(r[ee])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),D.then||Re(e),b.__e(D,e,n)}}else r==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):l=e.__e=mt(n.__e,e,n,a,o,r,i,g,c);return(f=b.diffed)&&f(e),128&e.__u?void 0:l}function Re(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Re))}function Ce(t,e,n){for(var a=0;a<n.length;a++)le(n[a],n[++a],n[++a]);b.__c&&b.__c(e,t),t.some(function(o){try{t=o.__h,o.__h=[],t.some(function(r){r.call(o)})}catch(r){b.__e(r,o.__v)}})}function $e(t){return typeof t!="object"||t==null||t.__b>0?t:Y(t)?t.map($e):t.constructor!==void 0?null:I({},t)}function mt(t,e,n,a,o,r,i,l,g){var c,f,p,s,u,v,k,d=n.props||U,_=e.props,m=e.type;if(m=="svg"?o="http://www.w3.org/2000/svg":m=="math"?o="http://www.w3.org/1998/Math/MathML":o||(o="http://www.w3.org/1999/xhtml"),r!=null){for(c=0;c<r.length;c++)if((u=r[c])&&"setAttribute"in u==!!m&&(m?u.localName==m:u.nodeType==3)){t=u,r[c]=null;break}}if(t==null){if(m==null)return document.createTextNode(_);t=document.createElementNS(o,m,_.is&&_),l&&(b.__m&&b.__m(e,r),l=!1),r=null}if(m==null)d===_||l&&t.data==_||(t.data=_);else{if(r=m=="textarea"&&_.defaultValue!=null?null:r&&V.call(t.childNodes),!l&&r!=null)for(d={},c=0;c<t.attributes.length;c++)d[(u=t.attributes[c]).name]=u.value;for(c in d)u=d[c],c=="dangerouslySetInnerHTML"?p=u:c=="children"||c in _||c=="value"&&"defaultValue"in _||c=="checked"&&"defaultChecked"in _||B(t,c,null,u,o);for(c in _)u=_[c],c=="children"?s=u:c=="dangerouslySetInnerHTML"?f=u:c=="value"?v=u:c=="checked"?k=u:l&&typeof u!="function"||d[c]===u||B(t,c,u,d[c],o);if(f)l||p&&(f.__html==p.__html||f.__html==t.innerHTML)||(t.innerHTML=f.__html),e.__k=[];else if(p&&(t.innerHTML=""),Ie(e.type=="template"?t.content:t,Y(s)?s:[s],e,n,a,m=="foreignObject"?"http://www.w3.org/1999/xhtml":o,r,i,r?r[0]:n.__k&&$(n,0),l,g),r!=null)for(c=r.length;c--;)ie(r[c]);l&&m!="textarea"||(c="value",m=="progress"&&v==null?t.removeAttribute("value"):v!=null&&(v!==t[c]||m=="progress"&&!v||m=="option"&&v!=d[c])&&B(t,c,v,d[c],o),c="checked",k!=null&&k!=t[c]&&B(t,c,k,d[c],o))}return t}function le(t,e,n){try{if(typeof t=="function"){var a=typeof t.__u=="function";a&&t.__u(),a&&e==null||(t.__u=t(e))}else t.current=e}catch(o){b.__e(o,n)}}function De(t,e,n){var a,o;if(b.unmount&&b.unmount(t),(a=t.ref)&&(a.current&&a.current!=t.__e||le(a,null,e)),(a=t.__c)!=null){if(a.componentWillUnmount)try{a.componentWillUnmount()}catch(r){b.__e(r,e)}a.base=a.__P=a.__n=null}if(a=t.__k)for(o=0;o<a.length;o++)a[o]&&De(a[o],e,n||typeof t.type!="function");n||ie(t.__e),t.__c=t.__=t.__e=void 0}function bt(t,e,n){return this.constructor(t,n)}function Ee(t,e,n){var a,o,r,i;e==document&&(e=document.documentElement),b.__&&b.__(t,e),o=(a=typeof n=="function")?null:n&&n.__k||e.__k,r=[],i=[],de(e,t=(!a&&n||e).__k=ut(P,null,[t]),o||U,U,e.namespaceURI,!a&&n?[n]:o?null:e.firstChild?V.call(e.childNodes):null,r,!a&&n?n:o?o.__e:e.firstChild,a,i),Ce(r,t,i),t.props.children=null}function Ge(t){function e(n){var a,o;return this.getChildContext||(a=new Set,(o={})[e.__c]=this,this.getChildContext=function(){return o},this.componentWillUnmount=function(){a=null},this.shouldComponentUpdate=function(r){this.props.value!=r.value&&a.forEach(function(i){i.__e=!0,oe(i)})},this.sub=function(r){a.add(r);var i=r.componentWillUnmount;r.componentWillUnmount=function(){a&&a.delete(r),i&&i.call(r)}}),n.children}return e.__c="__cC"+Pe++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,a){return n.children(a)}).contextType=e,e}V=q.slice,b={__e:function(t,e,n,a){for(var o,r,i;e=e.__;)if((o=e.__c)&&!o.__)try{if((r=o.constructor)&&r.getDerivedStateFromError!=null&&(o.setState(r.getDerivedStateFromError(t)),i=o.__d),o.componentDidCatch!=null&&(o.componentDidCatch(t,a||{}),i=o.__d),i)return o.__E=o}catch(l){t=l}throw t}},ye=0,gt=function(t){return t!=null&&t.constructor===void 0},F.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=I({},this.state),typeof t=="function"&&(t=t(I({},n),this.props)),t&&I(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),oe(this))},F.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),oe(this))},F.prototype.render=P,R=[],xe=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,we=function(t,e){return t.__v.__b-e.__v.__b},j.__r=0,ne=Math.random().toString(8),L="__d"+ne,M="__a"+ne,Se=/(PointerCapture)$|Capture$/i,se=0,ae=ke(!1),re=ke(!0),Pe=0;var G,y,ce,He,X=0,Fe=[],x=b,Me=x.__b,Ne=x.__r,Oe=x.diffed,We=x.__c,Be=x.unmount,Le=x.__;function Q(t,e){x.__h&&x.__h(y,t,X||e),X=0;var n=y.__H||(y.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function N(t){return X=1,vt(Ye,t)}function vt(t,e,n){var a=Q(G++,2);if(a.t=t,!a.__c&&(a.__=[n?n(e):Ye(void 0,e),function(l){var g=a.__N?a.__N[0]:a.__[0],c=a.t(g,l);g!==c&&(a.__N=[c,a.__[1]],a.__c.setState({}))}],a.__c=y,!y.__f)){var o=function(l,g,c){if(!a.__c.__H)return!0;var f=!1,p=a.__c.props!==l;if(a.__c.__H.__.some(function(u){if(u.__N){f=!0;var v=u.__[0];u.__=u.__N,u.__N=void 0,v!==u.__[0]&&(p=!0)}}),r){var s=r.call(this,l,g,c);return f?s||p:s}return!f||p};y.__f=!0;var r=y.shouldComponentUpdate,i=y.componentWillUpdate;y.componentWillUpdate=function(l,g,c){if(this.__e){var f=r;r=void 0,o(l,g,c),r=f}i&&i.call(this,l,g,c)},y.shouldComponentUpdate=o}return a.__N||a.__}function Ue(t,e){var n=Q(G++,4);!x.__s&&Ve(n.__H,e)&&(n.__=t,n.u=e,y.__h.push(n))}function qe(t){return X=5,kt(function(){return{current:t}},[])}function kt(t,e){var n=Q(G++,7);return Ve(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function je(t){var e=y.context[t.__c],n=Q(G++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(y)),e.props.value):t.__}function yt(){for(var t;t=Fe.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(K),e.__h.some(ge),e.__h=[]}catch(n){e.__h=[],x.__e(n,t.__v)}}}x.__b=function(t){y=null,Me&&Me(t)},x.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),Le&&Le(t,e)},x.__r=function(t){Ne&&Ne(t),G=0;var e=(y=t.__c).__H;e&&(ce===y?(e.__h=[],y.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(K),e.__h.some(ge),e.__h=[],G=0)),ce=y},x.diffed=function(t){Oe&&Oe(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(Fe.push(e)!==1&&He===x.requestAnimationFrame||((He=x.requestAnimationFrame)||xt)(yt)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),ce=y=null},x.__c=function(t,e){e.some(function(n){try{n.__h.some(K),n.__h=n.__h.filter(function(a){return!a.__||ge(a)})}catch(a){e.some(function(o){o.__h&&(o.__h=[])}),e=[],x.__e(a,n.__v)}}),We&&We(t,e)},x.unmount=function(t){Be&&Be(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(a){try{K(a)}catch(o){e=o}}),n.__H=void 0,e&&x.__e(e,n.__v))};var ze=typeof requestAnimationFrame=="function";function xt(t){var e,n=function(){clearTimeout(a),ze&&cancelAnimationFrame(e),setTimeout(t)},a=setTimeout(n,35);ze&&(e=requestAnimationFrame(n))}function K(t){var e=y,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),y=e}function ge(t){var e=y;t.__c=t.__(),y=e}function Ve(t,e){return!t||t.length!==e.length||e.some(function(n,a){return n!==t[a]})}function Ye(t,e){return typeof e=="function"?e(t):e}var Ke="(prefers-color-scheme: dark)";function wt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function St(t){let e=t;for(;e;){let n=wt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(Ke).matches?"dark":"light"}function Xe(t){let[e,n]=N("light");return Ue(()=>{let a=t.current;if(!a)return;let o=()=>n(St(a.parentElement||a));o();let r=new MutationObserver(o),i={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};r.observe(document.documentElement,i),document.body&&r.observe(document.body,i);let l=window.matchMedia?window.matchMedia(Ke):null;return l&&l.addEventListener("change",o),()=>{r.disconnect(),l&&l.removeEventListener("change",o)}},[t]),e}var w={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",edgeStrokeBlocked:2.75,edgeDashBlocked:"0.1 6",blockMarkR:6,edgeHitWidth:14,badgeRadius:13,badgeRadiusActive:15,diamondExtra:3,squareRadius:3,badgeOnEdgeMin:60,fragStroke:1.25,fragDash:"4 3",fragTagW:34,fragTagH:20,fragTagCut:6},Pt=`
  --dg-bg: #f6f8fa;
  --dg-border: #d0d7de;
  --dg-box: #ffffff;
  --dg-box-stroke: #8c959f;
  --dg-outline: #8c959f;
  --dg-text: #1f2328;
  --dg-text-sub: #57606a;
  --dg-on-solid: #e8edf2;
  --dg-on-solid-sub: #d5dbe1;
  --dg-edge: #6e7781;
  --dg-blocked: #e8590c;
  --dg-accent: #ffd43b;
  --dg-accent-stroke: #b8860b;
  --dg-on-accent: #1b2631;
  --dg-badge: #e6ebf1;
  --dg-badge-stroke: #6e7781;
  --dg-badge-text: #1f2328;
  --dg-panel: #ffffff;
  --dg-panel-title: #8a6500;
  --dg-panel-text: #1f2328;
  --dg-hint: #6e7781;

  --dg-danger: #c0392b;
  --dg-danger-deep: #7b241c;
  --dg-danger-tint: rgba(192, 57, 43, 0.08);
  --dg-safe: #27ae60;
  --dg-safe-deep: #145a32;
  --dg-safe-tint: rgba(39, 174, 96, 0.08);
  --dg-info: #2e86c1;
  --dg-info-deep: #1b4f72;
  --dg-info-tint: rgba(46, 134, 193, 0.08);
  --dg-solid-dark: #34495e;
  --dg-solid-dark-stroke: #1b2631;

  --dg-chip-info: #0b4f9c;
  --dg-chip-info-border: #7fb3ff;
  --dg-chip-info-bg: rgba(127, 179, 255, 0.15);
  --dg-chip-danger: #9c2a20;
  --dg-chip-danger-border: #ff8a80;
  --dg-chip-danger-bg: rgba(255, 138, 128, 0.15);
  --dg-chip-safe: #1e6b3a;
  --dg-chip-safe-border: #7ddba0;
  --dg-chip-safe-bg: rgba(125, 219, 160, 0.15);
  --dg-chip-neutral: #57606a;
  --dg-chip-neutral-border: #b6bec8;
  --dg-chip-neutral-bg: rgba(182, 190, 200, 0.15);
`,Qe=`
  --dg-bg: #10151c;
  --dg-border: #2c3644;
  --dg-box: #1e2530;
  --dg-box-stroke: #4a5568;
  --dg-outline: #5d6d7e;
  --dg-text: #e8edf2;
  --dg-text-sub: #a9b4c0;
  --dg-on-solid: #e8edf2;
  --dg-on-solid-sub: #d5dbe1;
  --dg-edge: #7f8c9a;
  --dg-blocked: #ff922b;
  --dg-accent: #f4d03f;
  --dg-accent-stroke: #b7950b;
  --dg-on-accent: #1b2631;
  --dg-badge: #2c3644;
  --dg-badge-stroke: #7f8c9a;
  --dg-badge-text: #e8edf2;
  --dg-panel: #161d27;
  --dg-panel-title: #f4d03f;
  --dg-panel-text: #cfd8e3;
  --dg-hint: #8b97a5;

  --dg-danger: #c0392b;
  --dg-danger-deep: #7b241c;
  --dg-danger-tint: rgba(192, 57, 43, 0.10);
  --dg-safe: #27ae60;
  --dg-safe-deep: #145a32;
  --dg-safe-tint: rgba(39, 174, 96, 0.10);
  --dg-info: #2e86c1;
  --dg-info-deep: #1b4f72;
  --dg-info-tint: rgba(46, 134, 193, 0.10);
  --dg-solid-dark: #1b2631;
  --dg-solid-dark-stroke: #5d6d7e;

  --dg-chip-info: #7fb3ff;
  --dg-chip-info-border: #2f5d8a;
  --dg-chip-info-bg: rgba(127, 179, 255, 0.08);
  --dg-chip-danger: #ff8a80;
  --dg-chip-danger-border: #8a3a33;
  --dg-chip-danger-bg: rgba(255, 138, 128, 0.08);
  --dg-chip-safe: #7ddba0;
  --dg-chip-safe-border: #2f6b45;
  --dg-chip-safe-bg: rgba(125, 219, 160, 0.08);
  --dg-chip-neutral: #a9b4c0;
  --dg-chip-neutral-border: #4a5568;
  --dg-chip-neutral-bg: rgba(169, 180, 192, 0.08);
`,Tt=`
  .dg-diagram { ${Pt} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${Qe} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${Qe} }

  /* Own the box model inside the component regardless of what the host page's
     stylesheet does with box-sizing elsewhere \u2014 the flex math below assumes
     border-box on every child. */
  .dg-diagram, .dg-diagram *, .dg-diagram *::before, .dg-diagram *::after {
    box-sizing: border-box;
  }

  /* frame: drawing and panel sit side by side, wrapping to a stack when the
     row can't fit both at a readable size */
  .dg-diagram {
    background: var(--dg-bg);
    border: 1px solid var(--dg-border);
    border-radius: 12px;
    padding: 12px;
    margin: 1.5em auto;
    max-width: 100%;
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 16px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  /* svg grows ~2px for every 1px the panel grows, so extra page width goes
     mostly to the drawing instead of stretching the text column past a
     readable line length. */
  .dg-diagram svg { flex: 2 1 680px; min-width: 380px; height: auto; display: block; }

  /* typography */
  .dg-label { fill: var(--dg-text); font-size: 13px; }
  .dg-label-bold { font-weight: 600; }
  .dg-label-sm { font-size: 11px; }
  .dg-label-sub { fill: var(--dg-text-sub); font-size: 11px; }
  .dg-label-outline { fill: var(--dg-text-sub); font-size: 11px; letter-spacing: 0.02em; text-transform: uppercase; }

  /* outlines: dashed regions grouping boxes */
  .dg-outline rect { fill: none; stroke: var(--dg-outline); }
  .dg-outline-danger rect { stroke: var(--dg-danger); }
  .dg-outline-safe rect { stroke: var(--dg-safe); }
  .dg-outline-info rect { stroke: var(--dg-info); }
  .dg-outline-filled.dg-outline-neutral rect { fill: var(--dg-box); }
  .dg-outline-filled.dg-outline-danger rect { fill: var(--dg-danger-tint); }
  .dg-outline-filled.dg-outline-safe rect { fill: var(--dg-safe-tint); }
  .dg-outline-filled.dg-outline-info rect { fill: var(--dg-info-tint); }

  /* combined fragments: solid frame, operator tag, dashed operand separators,
     guards in the tone of the branch they select */
  .dg-frag-frame { fill: none; stroke: var(--dg-box-stroke); }
  .dg-frag-tag { fill: var(--dg-box); stroke: var(--dg-box-stroke); }
  .dg-frag-sep { stroke: var(--dg-box-stroke); }
  .dg-frag-op { fill: var(--dg-text); font-size: 11px; font-weight: 700; }
  .dg-frag-guard { fill: var(--dg-text-sub); font-size: 11px; font-weight: 600; }
  .dg-frag-guard-safe { fill: var(--dg-chip-safe); }
  .dg-frag-guard-danger { fill: var(--dg-chip-danger); }

  /* boxes: components */
  .dg-box rect { fill: var(--dg-box); stroke: var(--dg-box-stroke); }
  .dg-box-solid-danger rect { fill: var(--dg-danger); stroke: var(--dg-danger-deep); }
  .dg-box-solid-safe rect { fill: var(--dg-safe-deep); stroke: var(--dg-safe); }
  .dg-box-solid-info rect { fill: var(--dg-info-deep); stroke: var(--dg-info); }
  .dg-box-solid-dark rect { fill: var(--dg-solid-dark); stroke: var(--dg-solid-dark-stroke); }
  .dg-box-solid .dg-label { fill: var(--dg-on-solid); }
  .dg-box-solid .dg-label-sub { fill: var(--dg-on-solid-sub); }

  /* edges */
  .dg-edge { stroke: var(--dg-edge); }
  .dg-edge-active { stroke: var(--dg-accent-stroke); }
  .dg-marker { fill: var(--dg-edge); }
  .dg-marker-active { fill: var(--dg-accent-stroke); }
  .dg-edge-blocked { stroke: var(--dg-blocked); stroke-linecap: round; }
  .dg-marker-blocked { stroke: var(--dg-blocked); }
  .dg-edge-blocked.dg-edge-active { stroke: var(--dg-accent-stroke); }
  .dg-marker-blocked-active { stroke: var(--dg-accent-stroke); }

  /* activity diagram (ActivityDiagram.jsx): node label, the file names inside
     a node (monospace; colour from dg-label-sub), the "changed" outline; the
     description of a step is in the shared detail panel */
  .dg-flow-label { font-size: 11.5px; font-weight: 600; }
  .dg-flow-node { cursor: pointer; }
  .dg-flow-node:focus { outline: none; }
  .dg-flow-changed rect { stroke: var(--dg-safe); stroke-width: 3; }
  .dg-flow-node-active rect, .dg-flow-node:focus rect { stroke: var(--dg-accent-stroke); stroke-width: 3; }
  .dg-flow-file { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 11px; }
  .dg-flow-edge-label { fill: var(--dg-text-sub); font-size: 11px; font-style: italic; }
  /* narrow screens: keep the drawing at a tappable size (boxes about 44px
     high) and let the frame scroll sideways; the panel stays under it */
  .dg-scroll-hint { display: none; margin-top: 4px; font-size: 12px; font-style: italic; color: var(--dg-hint); text-align: center; }
  @media (max-width: 720px) {
    .dg-scrollwrap { flex: 1 1 100% !important; min-width: 0 !important; max-width: 100%; }
    .dg-scroll { overflow-x: auto; }
    .dg-scroll svg { min-width: 800px; }
    .dg-scroll-hint { display: block; }
    .dg-diagram:has(.dg-flow-node) .dg-panel { position: static; flex-basis: 100%; min-width: 0; max-width: none; }
  }

  /* fluid drawing (Diagram \`fluid\`, the flow island): the frame is the size
     container, so the breakpoints follow the post column, not the viewport.
     Default (and any browser without container queries) = drawing full width
     with the panel below; it scales with the frame through its viewBox.
     720px..1239px: drawing and panel both 100% wide (panel below), the svg
     scales up as well as down (no width cap).
     >= 1240px: panel beside the drawing, fixed 300px; the drawing gets the
     rest of the row (>= ~900px).
     < 720px: drawing keeps 800px and scrolls sideways, swipe hint shown. */
  .dg-fluid { container-type: inline-size; }
  .dg-diagram.dg-fluid .dg-scrollwrap { flex: 1 1 100% !important; width: 100% !important; min-width: 0 !important; max-width: none !important; }
  .dg-diagram.dg-fluid .dg-scroll svg { width: 100%; min-width: 0; max-width: none; }
  .dg-diagram.dg-fluid .dg-panel { flex: 0 0 100% !important; width: 100% !important; min-width: 0 !important; max-width: none !important; position: static; max-height: none; }
  @container (min-width: 1240px) {
    .dg-diagram.dg-fluid .dg-scrollwrap { flex: 1 1 0 !important; width: auto !important; }
    .dg-diagram.dg-fluid .dg-panel { flex: 0 0 300px !important; width: 300px !important; min-width: 300px !important; max-width: 300px !important; position: sticky; max-height: calc(100vh - 32px); }
  }
  @container (max-width: 719px) {
    .dg-diagram.dg-fluid .dg-scroll { overflow-x: auto; }
    .dg-diagram.dg-fluid .dg-scroll svg { min-width: 800px; }
    .dg-diagram.dg-fluid .dg-scroll-hint { display: block; }
  }

  /* clickable edge with no badge of its own: a wide transparent stroke gives
     it a comfortable hit area without widening the visible line. */
  .dg-edge-hit { cursor: pointer; }
  .dg-edge-hit:focus { outline: none; }
  .dg-edge-hit:focus .dg-edge { stroke: var(--dg-accent-stroke); }
  .dg-edge-hit:focus .dg-edge-blocked { stroke: var(--dg-accent-stroke); }

  /* badges: the interactive hotspots */
  .dg-badge { cursor: pointer; }
  .dg-badge circle, .dg-badge polygon, .dg-badge rect { fill: var(--dg-badge); stroke: var(--dg-badge-stroke); }
  .dg-badge-diamond polygon { fill: var(--dg-box); stroke: var(--dg-info); }
  .dg-badge-square rect { fill: var(--dg-box); stroke: var(--dg-danger); }
  .dg-badge-related circle, .dg-badge-related polygon, .dg-badge-related rect { stroke: var(--dg-accent-stroke); stroke-width: 2.5; }
  .dg-badge-active circle, .dg-badge-active polygon, .dg-badge-active rect { fill: var(--dg-accent); stroke: var(--dg-accent-stroke); }
  .dg-badge:focus { outline: none; }
  .dg-badge:focus circle, .dg-badge:focus polygon, .dg-badge:focus rect { stroke: var(--dg-accent-stroke); stroke-width: 2.5; }
  .dg-badge-label { fill: var(--dg-badge-text); font-size: 11px; font-weight: 700; pointer-events: none; }
  .dg-badge-diamond .dg-badge-label { fill: var(--dg-chip-info); font-size: 10px; }
  .dg-badge-diamond.dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }
  .dg-badge-square .dg-badge-label { fill: var(--dg-chip-danger); font-size: 10px; }
  .dg-badge-square.dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }
  .dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }

  /* detail panel: beside the drawing when there's room, full-width below it
     once the row wraps. Sticky, so on a drawing taller than the viewport the
     panel follows the scroll until the diagram's bottom edge takes it away;
     capped at the viewport height and scrolls itself if its text runs longer.
     Once wrapped below the drawing the sticky has no effect. */
  .dg-diagram .dg-panel {
    box-sizing: border-box;
    flex: 1 1 320px;
    min-width: 280px;
    max-width: 440px;
    position: sticky;
    top: 16px;
    max-height: calc(100vh - 32px);
    overflow-y: auto;
    padding: 12px 14px;
    background: var(--dg-panel);
    border: 1px solid var(--dg-border);
    border-radius: 8px;
    min-height: 64px;
  }
  .dg-diagram .dg-panel-title { color: var(--dg-panel-title); font-weight: 700; font-size: 14px; margin-bottom: 4px; }
  .dg-diagram .dg-panel-desc { color: var(--dg-panel-text); font-size: 13.5px; line-height: 1.5; margin: 0 0 8px; white-space: pre-line; }
  .dg-diagram .dg-hint { color: var(--dg-hint); font-size: 13px; font-style: italic; margin: 0; }
  .dg-diagram .dg-chips { display: flex; flex-wrap: wrap; gap: 8px; }
  .dg-diagram .dg-chip { display: flex; align-items: baseline; gap: 5px; font-size: 11px; padding: 3px 10px; border-radius: 999px; border: 1px solid; }
  .dg-chip-id { font-weight: 700; }
  .dg-chip-desc { font-weight: 500; opacity: 0.85; }
  .dg-chip-info { color: var(--dg-chip-info); border-color: var(--dg-chip-info-border); background: var(--dg-chip-info-bg); }
  .dg-chip-danger { color: var(--dg-chip-danger); border-color: var(--dg-chip-danger-border); background: var(--dg-chip-danger-bg); }
  .dg-chip-safe { color: var(--dg-chip-safe); border-color: var(--dg-chip-safe-border); background: var(--dg-chip-safe-bg); }
  .dg-chip-neutral { color: var(--dg-chip-neutral); border-color: var(--dg-chip-neutral-border); background: var(--dg-chip-neutral-bg); }
`,Je="dg-diagram-styles";function Ze(){if(typeof document=="undefined"||document.getElementById(Je))return;let t=document.createElement("style");t.id=Je,t.textContent=Tt,document.head.appendChild(t)}var It=0,Vt=Array.isArray;function h(t,e,n,a,o,r){e||(e={});var i,l,g=e;if("ref"in g)for(l in g={},e)l=="ref"?i=e[l]:g[l]=e[l];var c={type:t,props:g,key:n,ref:i,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--It,__i:-1,__u:0,__source:o,__self:r};if(typeof t=="function"&&(i=t.defaultProps))for(l in i)g[l]===void 0&&(g[l]=i[l]);return b.vnode&&b.vnode(c),c}var nt=Ge({markerPrefix:"dg-arrow"});function J(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function At(t){let e=[],n=0;for(let o=1;o<t.length;o++){let[r,i]=t[o-1],[l,g]=t[o],c=Math.hypot(l-r,g-i);e.push(c),n+=c}let a=n/2;for(let o=0;o<e.length;o++){if(a<=e[o]||o===e.length-1){let[r,i]=t[o],[l,g]=t[o+1],c=e[o]===0?0:a/e[o];return[r+(l-r)*c,i+(g-i)*c]}a-=e[o]}return t[t.length-1]}function Rt(t,e="Edge"){for(let n=1;n<t.length;n++){let[a,o]=t[n-1],[r,i]=t[n];if(a!==r&&o!==i)throw new Error(`${e}: diagonal segment (${a},${o})\u2192(${r},${i}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function Z({x:t,y:e,w:n,h:a,label:o,variant:r="neutral",filled:i}){return h("g",{class:`dg-outline dg-outline-${r}${i?" dg-outline-filled":""}`,children:[h("rect",{x:t,y:e,width:n,height:a,rx:w.outlineRadius,"stroke-width":w.outlineStroke,"stroke-dasharray":w.outlineDash}),o?h("text",{x:t+14,y:e+22,class:"dg-label dg-label-outline",children:o}):null]})}function he({x:t,y:e,w:n,h:a,label:o,sub:r,variant:i="default",small:l}){let g=i.startsWith("solid-"),c=r?[].concat(r):[],f=l?14:18,p=c.length?e+a/2-6-(c.length-1)*14/2:e+a/2+5;return h("g",{class:`dg-box dg-box-${i}${g?" dg-box-solid":""}`,children:[h("rect",{x:t,y:e,width:n,height:a,rx:w.boxRadius,"stroke-width":w.boxStroke}),h("text",{x:t+n/2,y:p,"text-anchor":"middle",class:l?"dg-label dg-label-sm":"dg-label dg-label-bold",children:o}),c.map((s,u)=>h("text",{x:t+n/2,y:p+f+u*14,"text-anchor":"middle",class:"dg-label dg-label-sub",children:s},u))]})}function ue({points:t,dashed:e,blocked:n,active:a,dir:o,id:r,onSelect:i,onHover:l}){Rt(t,r?`Edge ${r}`:"Edge");let{markerPrefix:g}=je(nt),c=`${g}-${a?"active":"default"}`,f=o&&o!=="none",p=typeof i=="function",s=(d,_)=>{d.pointerType==="mouse"&&l&&l(_)},u=p?{role:"button",tabindex:0,"aria-label":r,onClick:()=>i(r),onPointerEnter:d=>s(d,r),onPointerLeave:d=>s(d,null),onKeyDown:d=>{(d.key==="Enter"||d.key===" ")&&(d.preventDefault(),i(r))}}:{},v=p?h("polyline",{points:J(t),fill:"none",stroke:"transparent","pointer-events":"stroke","stroke-width":w.edgeHitWidth}):null;if(n){let[d,_]=At(t),m=w.blockMarkR,S=`dg-marker-blocked${a?" dg-marker-blocked-active":""}`,A=h(P,{children:[h("polyline",{class:`dg-edge dg-edge-blocked${a?" dg-edge-active":""}`,points:J(t),fill:"none","stroke-width":w.edgeStrokeBlocked,"stroke-dasharray":w.edgeDashBlocked}),h("path",{d:`M${d-m},${_-m} L${d+m},${_+m} M${d+m},${_-m} L${d-m},${_+m}`,class:S,fill:"none","stroke-width":w.edgeStrokeBlocked,"stroke-linecap":"round"})]});return p?h("g",{class:"dg-edge-hit",...u,children:[v,A]}):A}let k=h("polyline",{class:`dg-edge${a?" dg-edge-active":""}`,points:J(t),fill:"none","stroke-width":a?w.edgeStrokeActive:w.edgeStroke,"stroke-dasharray":e?w.edgeDash:void 0,"marker-end":f?`url(#${c})`:void 0,"marker-start":o==="both"?`url(#${c})`:void 0});return p?h("g",{class:"dg-edge-hit",...u,children:[v,k]}):k}function at({id:t,label:e,name:n,x:a,y:o,active:r,related:i,pressed:l,shape:g="circle",onSelect:c,onHover:f}){let p=(k,d)=>{k.pointerType==="mouse"&&f&&f(d)},s=r?w.badgeRadiusActive:w.badgeRadius,u=s+w.diamondExtra,v=`dg-badge dg-badge-${g}${r?" dg-badge-active":i?" dg-badge-related":""}`;return h("g",{class:v,role:"button",tabindex:"0","aria-pressed":l,"aria-label":n?`${t}: ${n}`:t,onClick:()=>c(t),onPointerEnter:k=>p(k,t),onPointerLeave:k=>p(k,null),onKeyDown:k=>{(k.key==="Enter"||k.key===" ")&&(k.preventDefault(),c(t))},children:[g==="diamond"?h("polygon",{points:J([[a,o-u],[a+u,o],[a,o+u],[a-u,o]]),"stroke-width":"1.5"}):g==="square"?h("rect",{x:a-s,y:o-s,width:2*s,height:2*s,rx:w.squareRadius,"stroke-width":"1.5"}):h("circle",{cx:a,cy:o,r:s,"stroke-width":"1.5"}),h("text",{x:a,y:o+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function rt(){let[t,e]=N(null),[n,a]=N(null);return{shownId:n||t,pinnedId:t,select:r=>e(i=>i===r?null:r),setHover:a}}function Ct({tone:t="neutral",id:e,text:n}){return h("span",{class:`dg-chip dg-chip-${t}`,children:[h("span",{class:"dg-chip-id",children:e}),n?h("span",{class:"dg-chip-desc",children:n}):null]})}function ot({id:t,title:e,desc:n,chips:a,hint:o,children:r}){return h("div",{class:"dg-panel","aria-live":"polite",children:e?h(P,{children:[h("div",{class:"dg-panel-title",children:[t?h(P,{children:[t," \u2014 "]}):null,e]}),n?h("div",{class:"dg-panel-desc",children:n}):null,a&&a.length?h("div",{class:"dg-chips",children:a.map(i=>h(Ct,{...i},`${i.tone}-${i.id}`))}):null,r]}):h("div",{class:"dg-hint",children:o})})}function tt({id:t,active:e}){return h("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:h("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}var $t=680,Dt=380;function st({id:t,viewH:e,viewW:n=960,label:a,footer:o,scrollHint:r,fluid:i,children:l}){Ze();let g=qe(null),c=Xe(g),f=`dg-arrow-${t}`,p=n/960,s=p!==1&&!i?{flexBasis:`${Math.round($t*p)}px`,minWidth:`${Math.round(Dt*p)}px`}:void 0,u=h("svg",{viewBox:`0 0 ${n} ${e}`,style:r?void 0:s,role:"group","aria-label":a,children:[h("defs",{children:[h(tt,{id:`${f}-default`}),h(tt,{id:`${f}-active`,active:!0})]}),l]});return h("div",{class:i?"dg-diagram dg-fluid":"dg-diagram","data-dg-theme":c,ref:g,children:[h(nt.Provider,{value:{markerPrefix:f},children:r?h("div",{class:"dg-scrollwrap",style:s,children:[h("div",{class:"dg-scroll",children:u}),h("div",{class:"dg-scroll-hint",children:r})]}):u}),o]})}var O={"IP-1":"Syscalls","IP-2":"File access","IP-3":"Network","IP-4":"Gofer to host","IP-5":"Netstack to host","IP-6":"Runtime registration","AS-1":"Rootfs overlay","AS-2":"daemon.json",T1a:"Sentry escape",T1b:"Memory pressure","IP-R1":"Syscalls to host kernel","T-R1":"Kernel attack surface","T-R2":"Escape reaches the host","IP-G1":"Syscalls to Sentry","IP-G2":"Sentry to Gofer / Netstack","IP-G3":"Remaining host syscalls","T-G1":"Sentry bug","T-G2":"Residual host exposure"};var Et={ip:"circle",asset:"diamond",threat:"square"},Gt={ip:"neutral",asset:"info",threat:"danger"},Ht={safe:"solid-safe",info:"solid-info",danger:"solid-danger",neutral:"default"};function fe(t){return t.startsWith("IP-")?"ip":t.startsWith("AS-")?"asset":"threat"}function Mt(t,e){var a;let n=new Set(((a=t[e])==null?void 0:a.links)||[]);for(let[o,r]of Object.entries(t))(r.links||[]).includes(e)&&n.add(o);return n.delete(e),[...n]}function Nt(t){let e=t.status||{},n=l=>{let g=e[l];return g==null?null:typeof g=="string"?{status:g}:g},a=l=>{var g;return(g=n(l))==null?void 0:g.status},o=l=>(t.statusTone||{})[l]||"neutral",r=l=>(t.statusLabel||{})[l];return{entry:n,key:a,tone:o,label:r,chip:l=>(t.statusChip||{})[l]||r(l)}}function Ot({geom:t,title:e,st:n,keys:a}){let o=t.order||a;return h(P,{children:[e?h("text",{x:t.x,y:t.y+16,class:"dg-label dg-label-sub",children:e}):null,o.map((r,i)=>{let l=t.x+(t.titleW||0)+i*t.step;return h("g",{children:[h(Z,{x:l,y:t.y,w:t.swatch,h:t.swatch,variant:n.tone(r),filled:!0}),h("text",{x:l+t.swatch+8,y:t.y+16,class:"dg-label dg-label-sub",children:n.label(r)})]},`legend-${r}`)})]})}function pe({id:t,layout:e,text:n}){let{shownId:a,pinnedId:o,select:r,setHover:i}=rt(),l=a?n.items[a]:null,g=a?Mt(n.items,a):[],c=Nt(n),f=d=>{let _=c.key(d);return _?`${O[d]} \xB7 ${c.chip(_)}`:O[d]},p=a?c.entry(a):null,s=l?(l.reqs||[]).filter(d=>{var _;return(_=c.entry(d))==null?void 0:_.note}):[],u=["ip","asset","threat"],v=l?[...p?[{tone:c.tone(p.status),id:n.statusChipId||"Status",text:c.chip(p.status)}]:[],...g.slice().sort((d,_)=>u.indexOf(fe(d))-u.indexOf(fe(_))).map(d=>{var _;return{tone:Gt[fe(d)],id:d,text:O[d]||((_=n.items[d])==null?void 0:_.name)}}),...(l.goals||[]).map(d=>({tone:"neutral",id:d,text:O[d]})),...(l.reqs||[]).map(d=>({tone:"safe",id:d,text:f(d)}))]:[],k=[...e.ips.map(d=>({...d,kind:"ip"})),...e.assets.map(d=>({...d,kind:"asset"})),...e.threats.map(d=>({...d,kind:"threat"}))];return h(st,{id:t,viewH:e.viewH,viewW:e.viewW,label:n.label,footer:h(ot,{id:a,title:l==null?void 0:l.name,desc:l==null?void 0:l.desc,chips:v,hint:n.hint||"Circles are interaction points, diamonds assets, squares threat cases. Hover or click one to see what it connects to; the panel lists them with the capability goals (grey) and requirements (green) that hold there.",children:[l!=null&&l.code?h("div",{class:"dg-panel-desc",children:["Code: ",l.code]}):null,p!=null&&p.note?h("div",{class:"dg-panel-desc",children:p.note}):null,s.map(d=>h("div",{class:"dg-panel-desc",children:[h("strong",{children:d})," \u2014 ",c.entry(d).note]},`note-${d}`))]}),children:[e.outlines.map(d=>h(Z,{...d},d.label)),Object.entries(e.nodes).map(([d,_])=>h(he,{..._},d)),(e.statusBoxes||[]).map(d=>h(he,{x:d.x,y:d.y,w:d.w,h:d.h,small:d.small,label:d.id,sub:c.label(c.key(d.id)),variant:Ht[c.tone(c.key(d.id))]},`status-${d.id}`)),e.links.map((d,_)=>h(ue,{id:d.flow||`${t}-link-${_}`,points:d.points,dir:d.dir,dashed:d.dashed,blocked:d.blocked,active:d.flow?a===d.flow:void 0,onSelect:d.flow?r:void 0,onHover:d.flow?i:void 0},`l${_}`)),e.ips.map(d=>h(ue,{id:`${t}-${d.id}`,points:d.points,dir:d.dir,dashed:d.dashed,active:a===d.id},d.id)),e.statusHalo?k.filter(d=>c.key(d.id)).map(d=>h(Z,{x:d.at[0]-e.statusHalo,y:d.at[1]-e.statusHalo,w:2*e.statusHalo,h:2*e.statusHalo,variant:c.tone(c.key(d.id)),filled:!0},`halo-${d.id}`)):null,e.statusLegend?h(Ot,{geom:e.statusLegend,title:n.statusLegend,st:c,keys:Object.keys(n.statusLabel||{})}):null,k.map(d=>{var _;return h(at,{id:d.id,label:d.label,name:(_=n.items[d.id])==null?void 0:_.name,shape:Et[d.kind],x:d.at[0],y:d.at[1],active:a===d.id,pressed:o===d.id,onSelect:r,onHover:i},d.id)})]})}function it(t){let e=()=>{for(let[n,a]of t){let o=document.getElementById(n);o&&Ee(a(),o)}};document.readyState==="loading"?document.addEventListener("DOMContentLoaded",e):e()}var _e=(t,e)=>()=>h(pe,{id:t,layout:te[e].block,text:te[e].blockText});it([["island-gvisor-hardening",_e("gvisor-runtime-boundary","runtime")],["gvisor-capability-runc",_e("gvisor-cap-runc","runc")],["gvisor-capability-gvisor",_e("gvisor-cap-gvisor","gvisor")]]);})();
