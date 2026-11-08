(()=>{var pe={viewH:816,outlines:[{x:28,y:16,w:908,h:100,label:"Caller \xB7 create_agent_container, before arm"},{x:28,y:144,w:908,h:200,label:"Preconditions"},{x:28,y:372,w:172,h:200,label:"Clean slate",variant:"safe"},{x:212,y:372,w:356,h:200,label:"Identify and close",variant:"safe"},{x:580,y:372,w:356,h:200,label:"Controls and read-back",variant:"safe"},{x:28,y:600,w:540,h:200,label:"Release",variant:"danger"},{x:580,y:600,w:356,h:200,label:"Caller \xB7 after arm"}],nodes:{caller:{x:36,y:44,w:300,h:60,label:"Create, start, pause",sub:["docker create on agents-net, docker start","docker pause, then arm <container>"]},validate:{x:36,y:228,w:156,h:96,label:"Validate invocation",sub:["arm + valid container name","else usage text, exit 1"]},privilege:{x:220,y:228,w:156,h:96,label:"Privilege and engine",sub:["root, CAP_NET_ADMIN,","docker socket, engine"]},record:{x:404,y:228,w:156,h:96,label:"Record engine state",sub:["daemon.json SHA-256","engine start identity"]},network:{x:588,y:228,w:156,h:96,label:"Network and gate",sub:["agents-net exists","gate has the mark rule"]},paused:{x:772,y:228,w:156,h:96,label:"Require paused",sub:['runsc, restart "no"',"agents-net only","running and paused"]},clean:{x:36,y:456,w:156,h:96,label:"Clean slate",sub:["name-keyed disarm","leftovers removed first"]},identify:{x:220,y:456,w:156,h:96,label:"Identify host veth",sub:["PID, IP, MAC from inspect","veth read via nsenter","cross-checked"]},close:{x:404,y:456,w:156,h:96,label:"Close port",sub:["evict MAC from released","load as_port_<id>","policy drop, one nft -f -"]},apply:{x:588,y:456,w:156,h:96,label:"Apply controls",sub:["port flags, static FDB","neighbour entry","iptables rules"]},verify:{x:772,y:456,w:156,h:96,label:"Verify controls",sub:["read back: bridge, nft,","iptables; first mismatch","stops the run"]},release:{x:36,y:684,w:156,h:96,label:"Release port",sub:["one nft transaction:","marked accept rules +","MAC and name in released"],variant:"solid-dark"},engine:{x:220,y:684,w:156,h:96,label:"Verify engine state",sub:["daemon.json hash and","engine identity unchanged"]},report:{x:404,y:684,w:156,h:96,label:"Report armed",sub:["exit 0; the caller","may now unpause"]},check:{x:588,y:684,w:156,h:96,label:"Check, then unpause",sub:["check-gate (read only)","docker unpause"]},agent:{x:772,y:684,w:156,h:96,label:"Start the agent",sub:["docker exec -u 1001","only now"]}},links:[{points:[[192,276],[220,276]],dir:"end"},{points:[[376,276],[404,276]],dir:"end"},{points:[[560,276],[588,276]],dir:"end"},{points:[[744,276],[772,276]],dir:"end"},{points:[[192,504],[220,504]],dir:"end"},{points:[[376,504],[404,504]],dir:"end"},{points:[[560,504],[588,504]],dir:"end"},{points:[[744,504],[772,504]],dir:"end"},{points:[[192,732],[220,732]],dir:"end"},{points:[[376,732],[404,732]],dir:"end"},{points:[[560,732],[588,732]],dir:"end"},{points:[[744,732],[772,732]],dir:"end"}],ips:[{id:"IP-S1",label:"1",points:[[176,104],[176,228]],dir:"end",at:[176,196]},{id:"IP-S2",label:"2",points:[[298,180],[298,228]],dir:"end",at:[298,204]},{id:"IP-S3",label:"3",points:[[482,180],[482,228]],dir:"end",at:[482,204]},{id:"IP-S4",label:"4",points:[[666,180],[666,228]],dir:"end",at:[666,204]},{id:"IP-S5",label:"5",points:[[850,180],[850,228]],dir:"end",at:[850,204]},{id:"IP-S6",label:"6",points:[[850,324],[850,358],[176,358],[176,456]],dir:"end",at:[176,428]},{id:"IP-S7",label:"7",points:[[298,408],[298,456]],dir:"end",at:[298,432]},{id:"IP-S8",label:"8",points:[[482,408],[482,456]],dir:"end",at:[482,432]},{id:"IP-S9",label:"9",points:[[666,408],[666,456]],dir:"end",at:[666,432]},{id:"IP-S10",label:"10",points:[[850,408],[850,456]],dir:"end",at:[850,432]},{id:"IP-S11",label:"11",points:[[850,552],[850,586],[176,586],[176,684]],dir:"end",at:[176,656]},{id:"IP-S12",label:"12",points:[[298,636],[298,684]],dir:"end",at:[298,660]},{id:"IP-S13",label:"13",points:[[482,636],[482,684]],dir:"end",at:[482,660]}],assets:[],threats:[]},he={label:"Block diagram of the arm sequence in three rows read left to right: the caller creates, starts and pauses the container; arm runs 13 steps in five phases (preconditions, clean slate, identify and close, controls and read-back, release); the caller then runs check-gate, unpauses and starts the agent with docker exec",hint:"Numbered circles mark the 13 steps of arm, row by row; hover or click one to see what the step does. Phases are the dashed regions. Green: the container is paused and its port is closed. Red: the release, the last operation, which opens the port. The two boxes at the top and the two at the bottom right belong to the caller, not to arm.",items:{"IP-S1":{name:"Step 1: validate invocation",desc:["\u2022 Accepted: exactly arm followed by one container name.","\u2022 Name: must match ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ (a docker container name) and must give a non-empty port table id for as_port_<id>.","\u2022 Refused name: gate, because its rule tag agent-net:gate is the tag of the network gate rule.","\u2022 Anything else: prints the usage text, reports the not-done state GATED_UNARMED and exits with code 1, before anything is touched."].join(`
`),code:"agent-net: s01_validate_invocation.go (ValidateInvocation)",links:[]},"IP-S2":{name:"Step 2: check privilege and engine",desc:["\u2022 Checks: runs as root (euid 0).","\u2022 Checks: CAP_NET_ADMIN is in its effective capability set.","\u2022 Checks: it is in the agent host\u2019s own network namespace (the same as pid 1, where the bridge, nftables and iptables state lives).","\u2022 Checks: it can reach the Docker socket. All missing items are named together.","\u2022 Then: asks the Docker engine for its version.","\u2022 Failure: a missing privilege or an unreachable engine exits with code 1.","\u2022 Effect: it changes nothing."].join(`
`),code:"agent-net: s02_check_privilege_engine.go (CheckPrivilege)",links:[]},"IP-S3":{name:"Step 3: record engine state",desc:["\u2022 Records: the SHA-256 of /etc/docker/daemon.json and the Docker engine start identity.","\u2022 Use: step 12 compares both, so a change of the engine during the run is detected."].join(`
`),code:"agent-net: s05_record_engine_state.go (RecordEngineState)",links:[]},"IP-S4":{name:"Step 4: require agents-net and its gate",desc:["\u2022 Required: the network agents-net and its agents_gate table must exist, and the gate must have the mark rule.","\u2022 Mismatch: a gate or port rule without the mark counts as a mismatch.","\u2022 Consequence: a network created before the mark was added must be recreated."].join(`
`),code:"agent-net: s09_require_network.go (RequireNetwork)",links:[]},"IP-S5":{name:"Step 5: require a paused container",desc:["\u2022 Runtime: runsc.","\u2022 Restart policy: exactly no (an empty policy is refused).","\u2022 Network: attached to agents-net only.","\u2022 State: running and paused.","\u2022 Caller: arm does not create, start, pause or unpause the container; the caller does."].join(`
`),code:"agent-net: s34_require_paused.go (RequirePaused)",links:[]},"IP-S6":{name:"Step 6: clean slate (name-keyed disarm)",desc:["\u2022 Cleanup: the same name-keyed cleanup as disarm runs first, so arm always builds from an empty state.","\u2022 Removed: whatever an earlier container or run of that name left (gate elements, port table, tagged iptables rules, neighbour and FDB entries).","\u2022 Failure: a leftover that cannot be removed fails the arm."].join(`
`),code:"agent-net: s40_clean_slate.go (CleanSlate)",links:[]},"IP-S7":{name:"Step 7: identify the host veth",desc:["\u2022 Inputs: the container PID, IP and MAC, taken from the Docker inspect data.","\u2022 Veth: found from host state, the host kernel\u2019s view of the container\u2019s network namespace, read as root through nsenter.","\u2022 Trust: nothing reported by code inside the container is used.","\u2022 Cross-check: the veth against the agent MAC, the container IP and the agents-net bridge master.","\u2022 Failure: any mismatch fails the arm."].join(`
`),code:"agent-net: s12_identify_port.go (IdentifyPort)",links:[]},"IP-S8":{name:"Step 8: close the port (load the port table)",desc:["\u2022 Evict: the MAC is removed from the set released.","\u2022 Load: the initial port table as_port_<id>, with policy drop and no accept rule, in one atomic nft -f - transaction.","\u2022 Result: from here the port is closed at the veth as well as at the gate."].join(`
`),code:"agent-net: s10_close_port.go (ClosePort)",links:[]},"IP-S9":{name:"Step 9: apply the host-side controls",desc:["\u2022 Port flags on the veth: isolated on, learning off, locked on.","\u2022 Static FDB entry.","\u2022 Permanent neighbour entry on the bridge.","\u2022 iptables rules in DOCKER-USER and INPUT.","\u2022 Failure: the first failure stops the run with exit code 2."].join(`
`),code:"agent-net: s13_apply_controls.go (ApplyControls)",links:[]},"IP-S10":{name:"Step 10: verify the controls (read back)",desc:["\u2022 Read-back: each control (bridge -j link show, nft list table, iptables -S), compared with the specification.","\u2022 Failure: a mismatch stops the run with exit code 2, and the port stays closed."].join(`
`),code:"agent-net: s14_verify_controls.go (VerifyControls)",links:[]},"IP-S11":{name:"Step 11: release the port (one nft transaction)",desc:["\u2022 Re-check: the container is still running and paused.","\u2022 Transaction: one nft -f - transaction appends the accept rules to the container\u2019s port table (the ingress ones set the port mark).","\u2022 Transaction: the same transaction adds its MAC to the set released with the container name as the element comment.","\u2022 Failure: if the release or its read-back fails, closeAgain removes the MAC, loads the port closed and the tool exits with code 2."].join(`
`),code:"agent-net: s15_release_port.go (ReleasePort, closeAgain)",links:[]},"IP-S12":{name:"Step 12: verify the engine state",desc:["\u2022 Compared: the daemon.json hash and the Docker engine start identity, against the values recorded in step 3.","\u2022 Failure: a change is reported as a failure."].join(`
`),code:"agent-net: s35_verify_engine_unchanged.go (VerifyEngineUnchanged)",links:[]},"IP-S13":{name:"Step 13: report armed",desc:["\u2022 Arm: reports success and exits with code 0. The container is still paused.","\u2022 Caller, in order: runs check-gate, unpauses the container, and only after that starts the agent with docker exec -u 1001.","\u2022 Effect: in that order the agent never runs while the port is closed or unchecked."].join(`
`),code:"agent-net: s16_report_armed.go (ReportArmed); agent-sandbox: s22_release_container.go",links:[]}}};var V,b,ke,dt,A,me,xe,ye,ee,L,H,we,re,te,ne,Se,O={},W=[],lt=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,G=Array.isArray;function I(t,e){for(var n in e)t[n]=e[n];return t}function oe(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function ct(t,e,n){var a,o,r,s={};for(r in e)r=="key"?a=e[r]:r=="ref"?o=e[r]:s[r]=e[r];if(arguments.length>2&&(s.children=arguments.length>3?V.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(r in t.defaultProps)s[r]===void 0&&(s[r]=t.defaultProps[r]);return q(t,s,a,o,null)}function q(t,e,n,a,o){var r={type:t,props:e,key:n,ref:a,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:o==null?++ke:o,__i:-1,__u:0};return o==null&&b.vnode!=null&&b.vnode(r),r}function P(t){return t.children}function z(t,e){this.props=t,this.context=e}function D(t,e){if(e==null)return t.__?D(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?D(t):null}function gt(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,a=[],o=[],r=I({},e);r.__v=e.__v+1,b.vnode&&b.vnode(r),ie(t.__P,r,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,a,n==null?D(e):n,!!(32&e.__u),o),r.__v=e.__v,r.__.__k[r.__i]=r,Ae(a,r,o),e.__e=e.__=null,r.__e!=n&&Pe(r)}}function Pe(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),Pe(t)}function ae(t){(!t.__d&&(t.__d=!0)&&A.push(t)&&!U.__r++||me!=b.debounceRendering)&&((me=b.debounceRendering)||xe)(U)}function U(){try{for(var t,e=1;A.length;)A.length>e&&A.sort(ye),t=A.shift(),e=A.length,gt(t)}finally{A.length=U.__r=0}}function Ce(t,e,n,a,o,r,s,l,g,c,f){var p,i,_,v,k,d,h=a&&a.__k||W,m=e.length;for(g=ut(n,e,h,g,m),p=0;p<m;p++)(_=n.__k[p])!=null&&(i=_.__i!=-1&&h[_.__i]||O,_.__i=p,d=ie(t,_,i,o,r,s,l,g,c,f),v=_.__e,_.ref&&i.ref!=_.ref&&(i.ref&&se(i.ref,null,_),f.push(_.ref,_.__c||v,_)),k==null&&v!=null&&(k=v),4&_.__u?(g=Ie(_,g,t),i.__e&&(i.__e=null)):typeof _.type=="function"&&d!==void 0?g=d:v&&(g=v.nextSibling),_.__u&=-7);return n.__e=k,g}function ut(t,e,n,a,o){var r,s,l,g,c,f=n.length,p=f,i=0;for(t.__k=new Array(o),r=0;r<o;r++)(s=e[r])!=null&&typeof s!="boolean"&&typeof s!="function"?(typeof s=="string"||typeof s=="number"||typeof s=="bigint"||s.constructor==String?s=t.__k[r]=q(null,s,null,null,null):G(s)?s=t.__k[r]=q(P,{children:s},null,null,null):s.constructor===void 0&&s.__b>0?s=t.__k[r]=q(s.type,s.props,s.key,s.ref?s.ref:null,s.__v):t.__k[r]=s,g=r+i,s.__=t,s.__b=t.__b+1,l=null,(c=s.__i=_t(s,n,g,p))!=-1&&(p--,(l=n[c])&&(l.__u|=2)),l==null||l.__v==null?(c==-1&&(o>f?i--:o<f&&i++),typeof s.type!="function"&&(s.__u|=4)):c!=g&&(c==g-1?i--:c==g+1?i++:(c>g?i--:i++,s.__u|=4))):t.__k[r]=null;if(p)for(r=0;r<f;r++)(l=n[r])!=null&&!(2&l.__u)&&(l.__e==a&&(a=D(l)),De(l,l));return a}function Ie(t,e,n){var a,o;if(typeof t.type=="function"){for(a=t.__k,o=0;a&&o<a.length;o++)a[o]&&(a[o].__=t,e=Ie(a[o],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=D(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function _t(t,e,n,a){var o,r,s,l=t.key,g=t.type,c=e[n],f=c!=null&&(2&c.__u)==0;if(c===null&&l==null||f&&l==c.key&&g==c.type)return n;if(a>(f?1:0)){for(o=n-1,r=n+1;o>=0||r<e.length;)if((c=e[s=o>=0?o--:r++])!=null&&!(2&c.__u)&&l==c.key&&g==c.type)return s}return-1}function be(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||lt.test(e)?n:n+"px"}function B(t,e,n,a,o){var r,s;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof a=="string"&&(t.style.cssText=a=""),a)for(e in a)n&&e in n||be(t.style,e,"");if(n)for(e in n)a&&n[e]==a[e]||be(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")r=e!=(e=e.replace(we,"$1")),s=e.toLowerCase(),e=s in t||e=="onFocusOut"||e=="onFocusIn"?s.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+r]=n,n?a?n[H]=a[H]:(n[H]=re,t.addEventListener(e,r?ne:te,r)):t.removeEventListener(e,r?ne:te,r);else{if(o=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function ve(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[L]==null)e[L]=re++;else if(e[L]<n[H])return;return n(b.event?b.event(e):e)}}}function ie(t,e,n,a,o,r,s,l,g,c){var f,p,i,_,v,k,d,h,m,S,E,$,M,fe,F,J,C=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(g=!!(32&n.__u),r=[l=e.__e=n.__e]),(f=b.__b)&&f(e);e:if(typeof C=="function"){p=s.length;try{if(m=e.props,S=C.prototype&&C.prototype.render,E=(f=C.contextType)&&a[f.__c],$=f?E?E.props.value:f.__:a,n.__c?h=(i=e.__c=n.__c).__=i.__E:(S?e.__c=i=new C(m,$):(e.__c=i=new z(m,$),i.constructor=C,i.render=pt),E&&E.sub(i),i.state||(i.state={}),i.__n=a,_=i.__d=!0,i.__h=[],i._sb=[]),S&&i.__s==null&&(i.__s=i.state),S&&C.getDerivedStateFromProps!=null&&(i.__s==i.state&&(i.__s=I({},i.__s)),I(i.__s,C.getDerivedStateFromProps(m,i.__s))),v=i.props,k=i.state,i.__v=e,_)S&&C.getDerivedStateFromProps==null&&i.componentWillMount!=null&&i.componentWillMount(),S&&i.componentDidMount!=null&&i.__h.push(i.componentDidMount);else{if(S&&C.getDerivedStateFromProps==null&&m!==v&&i.componentWillReceiveProps!=null&&i.componentWillReceiveProps(m,$),e.__v==n.__v||!i.__e&&i.shouldComponentUpdate!=null&&i.shouldComponentUpdate(m,i.__s,$)===!1){e.__v!=n.__v&&(i.props=m,i.state=i.__s,i.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(R){R&&(R.__=e)}),W.push.apply(i.__h,i._sb),i._sb=[],i.__h.length&&s.push(i),l=D(n);break e}i.componentWillUpdate!=null&&i.componentWillUpdate(m,i.__s,$),S&&i.componentDidUpdate!=null&&i.__h.push(function(){i.componentDidUpdate(v,k,d)})}if(i.context=$,i.props=m,i.__P=t,i.__e=!1,M=b.__r,fe=0,S)i.state=i.__s,i.__d=!1,M&&M(e),f=i.render(i.props,i.state,i.context),W.push.apply(i.__h,i._sb),i._sb=[];else do i.__d=!1,M&&M(e),f=i.render(i.props,i.state,i.context),i.state=i.__s;while(i.__d&&++fe<25);i.state=i.__s,i.getChildContext!=null&&(a=I(I({},a),i.getChildContext())),S&&!_&&i.getSnapshotBeforeUpdate!=null&&(d=i.getSnapshotBeforeUpdate(v,k)),F=f!=null&&f.type===P&&f.key==null?$e(f.props.children):f,l=Ce(t,G(F)?F:[F],e,n,a,o,r,s,l,g,c),i.base=e.__e,e.__u&=-161,i.__h.length&&s.push(i),h&&(i.__E=i.__=null)}catch(R){if(s.length=p,e.__v=null,g||r!=null){if(R.then){for(e.__u|=g?160:128;l&&l.nodeType==8&&l.nextSibling;)l=l.nextSibling;r!=null&&(r[r.indexOf(l)]=null),e.__e=l}else if(r!=null)for(J=r.length;J--;)oe(r[J])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),R.then||Ee(e),b.__e(R,e,n)}}else r==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):l=e.__e=ft(n.__e,e,n,a,o,r,s,g,c);return(f=b.diffed)&&f(e),128&e.__u?void 0:l}function Ee(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Ee))}function Ae(t,e,n){for(var a=0;a<n.length;a++)se(n[a],n[++a],n[++a]);b.__c&&b.__c(e,t),t.some(function(o){try{t=o.__h,o.__h=[],t.some(function(r){r.call(o)})}catch(r){b.__e(r,o.__v)}})}function $e(t){return typeof t!="object"||t==null||t.__b>0?t:G(t)?t.map($e):t.constructor!==void 0?null:I({},t)}function ft(t,e,n,a,o,r,s,l,g){var c,f,p,i,_,v,k,d=n.props||O,h=e.props,m=e.type;if(m=="svg"?o="http://www.w3.org/2000/svg":m=="math"?o="http://www.w3.org/1998/Math/MathML":o||(o="http://www.w3.org/1999/xhtml"),r!=null){for(c=0;c<r.length;c++)if((_=r[c])&&"setAttribute"in _==!!m&&(m?_.localName==m:_.nodeType==3)){t=_,r[c]=null;break}}if(t==null){if(m==null)return document.createTextNode(h);t=document.createElementNS(o,m,h.is&&h),l&&(b.__m&&b.__m(e,r),l=!1),r=null}if(m==null)d===h||l&&t.data==h||(t.data=h);else{if(r=m=="textarea"&&h.defaultValue!=null?null:r&&V.call(t.childNodes),!l&&r!=null)for(d={},c=0;c<t.attributes.length;c++)d[(_=t.attributes[c]).name]=_.value;for(c in d)_=d[c],c=="dangerouslySetInnerHTML"?p=_:c=="children"||c in h||c=="value"&&"defaultValue"in h||c=="checked"&&"defaultChecked"in h||B(t,c,null,_,o);for(c in h)_=h[c],c=="children"?i=_:c=="dangerouslySetInnerHTML"?f=_:c=="value"?v=_:c=="checked"?k=_:l&&typeof _!="function"||d[c]===_||B(t,c,_,d[c],o);if(f)l||p&&(f.__html==p.__html||f.__html==t.innerHTML)||(t.innerHTML=f.__html),e.__k=[];else if(p&&(t.innerHTML=""),Ce(e.type=="template"?t.content:t,G(i)?i:[i],e,n,a,m=="foreignObject"?"http://www.w3.org/1999/xhtml":o,r,s,r?r[0]:n.__k&&D(n,0),l,g),r!=null)for(c=r.length;c--;)oe(r[c]);l&&m!="textarea"||(c="value",m=="progress"&&v==null?t.removeAttribute("value"):v!=null&&(v!==t[c]||m=="progress"&&!v||m=="option"&&v!=d[c])&&B(t,c,v,d[c],o),c="checked",k!=null&&k!=t[c]&&B(t,c,k,d[c],o))}return t}function se(t,e,n){try{if(typeof t=="function"){var a=typeof t.__u=="function";a&&t.__u(),a&&e==null||(t.__u=t(e))}else t.current=e}catch(o){b.__e(o,n)}}function De(t,e,n){var a,o;if(b.unmount&&b.unmount(t),(a=t.ref)&&(a.current&&a.current!=t.__e||se(a,null,e)),(a=t.__c)!=null){if(a.componentWillUnmount)try{a.componentWillUnmount()}catch(r){b.__e(r,e)}a.base=a.__P=a.__n=null}if(a=t.__k)for(o=0;o<a.length;o++)a[o]&&De(a[o],e,n||typeof t.type!="function");n||oe(t.__e),t.__c=t.__=t.__e=void 0}function pt(t,e,n){return this.constructor(t,n)}function Re(t,e,n){var a,o,r,s;e==document&&(e=document.documentElement),b.__&&b.__(t,e),o=(a=typeof n=="function")?null:n&&n.__k||e.__k,r=[],s=[],ie(e,t=(!a&&n||e).__k=ct(P,null,[t]),o||O,O,e.namespaceURI,!a&&n?[n]:o?null:e.firstChild?V.call(e.childNodes):null,r,!a&&n?n:o?o.__e:e.firstChild,a,s),Ae(r,t,s),t.props.children=null}function Te(t){function e(n){var a,o;return this.getChildContext||(a=new Set,(o={})[e.__c]=this,this.getChildContext=function(){return o},this.componentWillUnmount=function(){a=null},this.shouldComponentUpdate=function(r){this.props.value!=r.value&&a.forEach(function(s){s.__e=!0,ae(s)})},this.sub=function(r){a.add(r);var s=r.componentWillUnmount;r.componentWillUnmount=function(){a&&a.delete(r),s&&s.call(r)}}),n.children}return e.__c="__cC"+Se++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,a){return n.children(a)}).contextType=e,e}V=W.slice,b={__e:function(t,e,n,a){for(var o,r,s;e=e.__;)if((o=e.__c)&&!o.__)try{if((r=o.constructor)&&r.getDerivedStateFromError!=null&&(o.setState(r.getDerivedStateFromError(t)),s=o.__d),o.componentDidCatch!=null&&(o.componentDidCatch(t,a||{}),s=o.__d),s)return o.__E=o}catch(l){t=l}throw t}},ke=0,dt=function(t){return t!=null&&t.constructor===void 0},z.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=I({},this.state),typeof t=="function"&&(t=t(I({},n),this.props)),t&&I(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),ae(this))},z.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),ae(this))},z.prototype.render=P,A=[],xe=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,ye=function(t,e){return t.__v.__b-e.__v.__b},U.__r=0,ee=Math.random().toString(8),L="__d"+ee,H="__a"+ee,we=/(PointerCapture)$|Capture$/i,re=0,te=ve(!1),ne=ve(!0),Se=0;var T,x,de,Me,Y=0,ze=[],y=b,He=y.__b,Ne=y.__r,je=y.diffed,Fe=y.__c,Be=y.unmount,Le=y.__;function X(t,e){y.__h&&y.__h(x,t,Y||e),Y=0;var n=x.__H||(x.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function N(t){return Y=1,ht(Ge,t)}function ht(t,e,n){var a=X(T++,2);if(a.t=t,!a.__c&&(a.__=[n?n(e):Ge(void 0,e),function(l){var g=a.__N?a.__N[0]:a.__[0],c=a.t(g,l);g!==c&&(a.__N=[c,a.__[1]],a.__c.setState({}))}],a.__c=x,!x.__f)){var o=function(l,g,c){if(!a.__c.__H)return!0;var f=!1,p=a.__c.props!==l;if(a.__c.__H.__.some(function(_){if(_.__N){f=!0;var v=_.__[0];_.__=_.__N,_.__N=void 0,v!==_.__[0]&&(p=!0)}}),r){var i=r.call(this,l,g,c);return f?i||p:i}return!f||p};x.__f=!0;var r=x.shouldComponentUpdate,s=x.componentWillUpdate;x.componentWillUpdate=function(l,g,c){if(this.__e){var f=r;r=void 0,o(l,g,c),r=f}s&&s.call(this,l,g,c)},x.shouldComponentUpdate=o}return a.__N||a.__}function Oe(t,e){var n=X(T++,4);!y.__s&&Ve(n.__H,e)&&(n.__=t,n.u=e,x.__h.push(n))}function We(t){return Y=5,mt(function(){return{current:t}},[])}function mt(t,e){var n=X(T++,7);return Ve(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function Ue(t){var e=x.context[t.__c],n=X(T++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(x)),e.props.value):t.__}function bt(){for(var t;t=ze.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(K),e.__h.some(le),e.__h=[]}catch(n){e.__h=[],y.__e(n,t.__v)}}}y.__b=function(t){x=null,He&&He(t)},y.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),Le&&Le(t,e)},y.__r=function(t){Ne&&Ne(t),T=0;var e=(x=t.__c).__H;e&&(de===x?(e.__h=[],x.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(K),e.__h.some(le),e.__h=[],T=0)),de=x},y.diffed=function(t){je&&je(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(ze.push(e)!==1&&Me===y.requestAnimationFrame||((Me=y.requestAnimationFrame)||vt)(bt)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),de=x=null},y.__c=function(t,e){e.some(function(n){try{n.__h.some(K),n.__h=n.__h.filter(function(a){return!a.__||le(a)})}catch(a){e.some(function(o){o.__h&&(o.__h=[])}),e=[],y.__e(a,n.__v)}}),Fe&&Fe(t,e)},y.unmount=function(t){Be&&Be(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(a){try{K(a)}catch(o){e=o}}),n.__H=void 0,e&&y.__e(e,n.__v))};var qe=typeof requestAnimationFrame=="function";function vt(t){var e,n=function(){clearTimeout(a),qe&&cancelAnimationFrame(e),setTimeout(t)},a=setTimeout(n,35);qe&&(e=requestAnimationFrame(n))}function K(t){var e=x,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),x=e}function le(t){var e=x;t.__c=t.__(),x=e}function Ve(t,e){return!t||t.length!==e.length||e.some(function(n,a){return n!==t[a]})}function Ge(t,e){return typeof e=="function"?e(t):e}var Ke="(prefers-color-scheme: dark)";function kt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function xt(t){let e=t;for(;e;){let n=kt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(Ke).matches?"dark":"light"}function Ye(t){let[e,n]=N("light");return Oe(()=>{let a=t.current;if(!a)return;let o=()=>n(xt(a.parentElement||a));o();let r=new MutationObserver(o),s={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};r.observe(document.documentElement,s),document.body&&r.observe(document.body,s);let l=window.matchMedia?window.matchMedia(Ke):null;return l&&l.addEventListener("change",o),()=>{r.disconnect(),l&&l.removeEventListener("change",o)}},[t]),e}var w={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",edgeStrokeBlocked:2.75,edgeDashBlocked:"0.1 6",blockMarkR:6,edgeHitWidth:14,badgeRadius:13,badgeRadiusActive:15,diamondExtra:3,squareRadius:3,badgeOnEdgeMin:60,fragStroke:1.25,fragDash:"4 3",fragTagW:34,fragTagH:20,fragTagCut:6},yt=`
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
`,Xe=`
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
`,wt=`
  .dg-diagram { ${yt} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${Xe} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${Xe} }

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
`,Ze="dg-diagram-styles";function Qe(){if(typeof document=="undefined"||document.getElementById(Ze))return;let t=document.createElement("style");t.id=Ze,t.textContent=wt,document.head.appendChild(t)}var St=0,Ot=Array.isArray;function u(t,e,n,a,o,r){e||(e={});var s,l,g=e;if("ref"in g)for(l in g={},e)l=="ref"?s=e[l]:g[l]=e[l];var c={type:t,props:g,key:n,ref:s,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--St,__i:-1,__u:0,__source:o,__self:r};if(typeof t=="function"&&(s=t.defaultProps))for(l in s)g[l]===void 0&&(g[l]=s[l]);return b.vnode&&b.vnode(c),c}var tt=Te({markerPrefix:"dg-arrow"});function Z(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function Pt(t){let e=[],n=0;for(let o=1;o<t.length;o++){let[r,s]=t[o-1],[l,g]=t[o],c=Math.hypot(l-r,g-s);e.push(c),n+=c}let a=n/2;for(let o=0;o<e.length;o++){if(a<=e[o]||o===e.length-1){let[r,s]=t[o],[l,g]=t[o+1],c=e[o]===0?0:a/e[o];return[r+(l-r)*c,s+(g-s)*c]}a-=e[o]}return t[t.length-1]}function Ct(t,e="Edge"){for(let n=1;n<t.length;n++){let[a,o]=t[n-1],[r,s]=t[n];if(a!==r&&o!==s)throw new Error(`${e}: diagonal segment (${a},${o})\u2192(${r},${s}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function Q({x:t,y:e,w:n,h:a,label:o,variant:r="neutral",filled:s}){return u("g",{class:`dg-outline dg-outline-${r}${s?" dg-outline-filled":""}`,children:[u("rect",{x:t,y:e,width:n,height:a,rx:w.outlineRadius,"stroke-width":w.outlineStroke,"stroke-dasharray":w.outlineDash}),o?u("text",{x:t+14,y:e+22,class:"dg-label dg-label-outline",children:o}):null]})}function ce({x:t,y:e,w:n,h:a,label:o,sub:r,variant:s="default",small:l}){let g=s.startsWith("solid-"),c=r?[].concat(r):[],f=l?14:18,p=c.length?e+a/2-6-(c.length-1)*14/2:e+a/2+5;return u("g",{class:`dg-box dg-box-${s}${g?" dg-box-solid":""}`,children:[u("rect",{x:t,y:e,width:n,height:a,rx:w.boxRadius,"stroke-width":w.boxStroke}),u("text",{x:t+n/2,y:p,"text-anchor":"middle",class:l?"dg-label dg-label-sm":"dg-label dg-label-bold",children:o}),c.map((i,_)=>u("text",{x:t+n/2,y:p+f+_*14,"text-anchor":"middle",class:"dg-label dg-label-sub",children:i},_))]})}function ge({points:t,dashed:e,blocked:n,active:a,dir:o,id:r,onSelect:s,onHover:l}){Ct(t,r?`Edge ${r}`:"Edge");let{markerPrefix:g}=Ue(tt),c=`${g}-${a?"active":"default"}`,f=o&&o!=="none",p=typeof s=="function",i=(d,h)=>{d.pointerType==="mouse"&&l&&l(h)},_=p?{role:"button",tabindex:0,"aria-label":r,onClick:()=>s(r),onPointerEnter:d=>i(d,r),onPointerLeave:d=>i(d,null),onKeyDown:d=>{(d.key==="Enter"||d.key===" ")&&(d.preventDefault(),s(r))}}:{},v=p?u("polyline",{points:Z(t),fill:"none",stroke:"transparent","pointer-events":"stroke","stroke-width":w.edgeHitWidth}):null;if(n){let[d,h]=Pt(t),m=w.blockMarkR,S=`dg-marker-blocked${a?" dg-marker-blocked-active":""}`,E=u(P,{children:[u("polyline",{class:`dg-edge dg-edge-blocked${a?" dg-edge-active":""}`,points:Z(t),fill:"none","stroke-width":w.edgeStrokeBlocked,"stroke-dasharray":w.edgeDashBlocked}),u("path",{d:`M${d-m},${h-m} L${d+m},${h+m} M${d+m},${h-m} L${d-m},${h+m}`,class:S,fill:"none","stroke-width":w.edgeStrokeBlocked,"stroke-linecap":"round"})]});return p?u("g",{class:"dg-edge-hit",..._,children:[v,E]}):E}let k=u("polyline",{class:`dg-edge${a?" dg-edge-active":""}`,points:Z(t),fill:"none","stroke-width":a?w.edgeStrokeActive:w.edgeStroke,"stroke-dasharray":e?w.edgeDash:void 0,"marker-end":f?`url(#${c})`:void 0,"marker-start":o==="both"?`url(#${c})`:void 0});return p?u("g",{class:"dg-edge-hit",..._,children:[v,k]}):k}function nt({id:t,label:e,name:n,x:a,y:o,active:r,related:s,pressed:l,shape:g="circle",onSelect:c,onHover:f}){let p=(k,d)=>{k.pointerType==="mouse"&&f&&f(d)},i=r?w.badgeRadiusActive:w.badgeRadius,_=i+w.diamondExtra,v=`dg-badge dg-badge-${g}${r?" dg-badge-active":s?" dg-badge-related":""}`;return u("g",{class:v,role:"button",tabindex:"0","aria-pressed":l,"aria-label":n?`${t}: ${n}`:t,onClick:()=>c(t),onPointerEnter:k=>p(k,t),onPointerLeave:k=>p(k,null),onKeyDown:k=>{(k.key==="Enter"||k.key===" ")&&(k.preventDefault(),c(t))},children:[g==="diamond"?u("polygon",{points:Z([[a,o-_],[a+_,o],[a,o+_],[a-_,o]]),"stroke-width":"1.5"}):g==="square"?u("rect",{x:a-i,y:o-i,width:2*i,height:2*i,rx:w.squareRadius,"stroke-width":"1.5"}):u("circle",{cx:a,cy:o,r:i,"stroke-width":"1.5"}),u("text",{x:a,y:o+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function at(){let[t,e]=N(null),[n,a]=N(null);return{shownId:n||t,pinnedId:t,select:r=>e(s=>s===r?null:r),setHover:a}}function It({tone:t="neutral",id:e,text:n}){return u("span",{class:`dg-chip dg-chip-${t}`,children:[u("span",{class:"dg-chip-id",children:e}),n?u("span",{class:"dg-chip-desc",children:n}):null]})}function rt({id:t,title:e,desc:n,chips:a,hint:o,children:r}){return u("div",{class:"dg-panel","aria-live":"polite",children:e?u(P,{children:[u("div",{class:"dg-panel-title",children:[t?u(P,{children:[t," \u2014 "]}):null,e]}),n?u("div",{class:"dg-panel-desc",children:n}):null,a&&a.length?u("div",{class:"dg-chips",children:a.map(s=>u(It,{...s},`${s.tone}-${s.id}`))}):null,r]}):u("div",{class:"dg-hint",children:o})})}function et({id:t,active:e}){return u("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:u("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}var Et=680,At=380;function ot({id:t,viewH:e,viewW:n=960,label:a,footer:o,scrollHint:r,fluid:s,children:l}){Qe();let g=We(null),c=Ye(g),f=`dg-arrow-${t}`,p=n/960,i=p!==1&&!s?{flexBasis:`${Math.round(Et*p)}px`,minWidth:`${Math.round(At*p)}px`}:void 0,_=u("svg",{viewBox:`0 0 ${n} ${e}`,style:r?void 0:i,role:"group","aria-label":a,children:[u("defs",{children:[u(et,{id:`${f}-default`}),u(et,{id:`${f}-active`,active:!0})]}),l]});return u("div",{class:s?"dg-diagram dg-fluid":"dg-diagram","data-dg-theme":c,ref:g,children:[u(tt.Provider,{value:{markerPrefix:f},children:r?u("div",{class:"dg-scrollwrap",style:i,children:[u("div",{class:"dg-scroll",children:_}),u("div",{class:"dg-scroll-hint",children:r})]}):_}),o]})}var j={"IP-1":"Host boundary","IP-2":"Per-port table","IP-3":"Port controls","IP-4":"agents_gate","IP-5":"iptables rules","IP-6":"Public internet","IP-S1":"Validate invocation","IP-S2":"Privilege and engine","IP-S3":"Record engine state","IP-S4":"Network and gate","IP-S5":"Require paused","IP-S6":"Clean slate","IP-S7":"Identify veth","IP-S8":"Close port","IP-S9":"Apply controls","IP-S10":"Verify controls","IP-S11":"Release port","IP-S12":"Verify engine state","IP-S13":"Report armed"};var $t={ip:"circle",asset:"diamond",threat:"square"},Dt={ip:"neutral",asset:"info",threat:"danger"},Rt={safe:"solid-safe",info:"solid-info",danger:"solid-danger",neutral:"default"};function ue(t){return t.startsWith("IP-")?"ip":t.startsWith("AS-")?"asset":"threat"}function Tt(t,e){var a;let n=new Set(((a=t[e])==null?void 0:a.links)||[]);for(let[o,r]of Object.entries(t))(r.links||[]).includes(e)&&n.add(o);return n.delete(e),[...n]}function Mt(t){let e=t.status||{},n=l=>{let g=e[l];return g==null?null:typeof g=="string"?{status:g}:g},a=l=>{var g;return(g=n(l))==null?void 0:g.status},o=l=>(t.statusTone||{})[l]||"neutral",r=l=>(t.statusLabel||{})[l];return{entry:n,key:a,tone:o,label:r,chip:l=>(t.statusChip||{})[l]||r(l)}}function Ht({geom:t,title:e,st:n,keys:a}){let o=t.order||a;return u(P,{children:[e?u("text",{x:t.x,y:t.y+16,class:"dg-label dg-label-sub",children:e}):null,o.map((r,s)=>{let l=t.x+(t.titleW||0)+s*t.step;return u("g",{children:[u(Q,{x:l,y:t.y,w:t.swatch,h:t.swatch,variant:n.tone(r),filled:!0}),u("text",{x:l+t.swatch+8,y:t.y+16,class:"dg-label dg-label-sub",children:n.label(r)})]},`legend-${r}`)})]})}function _e({id:t,layout:e,text:n}){let{shownId:a,pinnedId:o,select:r,setHover:s}=at(),l=a?n.items[a]:null,g=a?Tt(n.items,a):[],c=Mt(n),f=d=>{let h=c.key(d);return h?`${j[d]} \xB7 ${c.chip(h)}`:j[d]},p=a?c.entry(a):null,i=l?(l.reqs||[]).filter(d=>{var h;return(h=c.entry(d))==null?void 0:h.note}):[],_=["ip","asset","threat"],v=l?[...p?[{tone:c.tone(p.status),id:n.statusChipId||"Status",text:c.chip(p.status)}]:[],...g.slice().sort((d,h)=>_.indexOf(ue(d))-_.indexOf(ue(h))).map(d=>{var h;return{tone:Dt[ue(d)],id:d,text:j[d]||((h=n.items[d])==null?void 0:h.name)}}),...(l.goals||[]).map(d=>({tone:"neutral",id:d,text:j[d]})),...(l.reqs||[]).map(d=>({tone:"safe",id:d,text:f(d)}))]:[],k=[...e.ips.map(d=>({...d,kind:"ip"})),...e.assets.map(d=>({...d,kind:"asset"})),...e.threats.map(d=>({...d,kind:"threat"}))];return u(ot,{id:t,viewH:e.viewH,viewW:e.viewW,label:n.label,footer:u(rt,{id:a,title:l==null?void 0:l.name,desc:l==null?void 0:l.desc,chips:v,hint:n.hint||"Circles are interaction points, diamonds assets, squares threat cases. Hover or click one to see what it connects to; the panel lists them with the capability goals (grey) and requirements (green) that hold there.",children:[l!=null&&l.code?u("div",{class:"dg-panel-desc",children:["Code: ",l.code]}):null,p!=null&&p.note?u("div",{class:"dg-panel-desc",children:p.note}):null,i.map(d=>u("div",{class:"dg-panel-desc",children:[u("strong",{children:d})," \u2014 ",c.entry(d).note]},`note-${d}`))]}),children:[e.outlines.map(d=>u(Q,{...d},d.label)),Object.entries(e.nodes).map(([d,h])=>u(ce,{...h},d)),(e.statusBoxes||[]).map(d=>u(ce,{x:d.x,y:d.y,w:d.w,h:d.h,small:d.small,label:d.id,sub:c.label(c.key(d.id)),variant:Rt[c.tone(c.key(d.id))]},`status-${d.id}`)),e.links.map((d,h)=>u(ge,{id:d.flow||`${t}-link-${h}`,points:d.points,dir:d.dir,dashed:d.dashed,blocked:d.blocked,active:d.flow?a===d.flow:void 0,onSelect:d.flow?r:void 0,onHover:d.flow?s:void 0},`l${h}`)),e.ips.map(d=>u(ge,{id:`${t}-${d.id}`,points:d.points,dir:d.dir,dashed:d.dashed,active:a===d.id},d.id)),e.statusHalo?k.filter(d=>c.key(d.id)).map(d=>u(Q,{x:d.at[0]-e.statusHalo,y:d.at[1]-e.statusHalo,w:2*e.statusHalo,h:2*e.statusHalo,variant:c.tone(c.key(d.id)),filled:!0},`halo-${d.id}`)):null,e.statusLegend?u(Ht,{geom:e.statusLegend,title:n.statusLegend,st:c,keys:Object.keys(n.statusLabel||{})}):null,k.map(d=>{var h;return u(nt,{id:d.id,label:d.label,name:(h=n.items[d.id])==null?void 0:h.name,shape:$t[d.kind],x:d.at[0],y:d.at[1],active:a===d.id,pressed:o===d.id,onSelect:r,onHover:s},d.id)})]})}function it(t){let e=()=>{for(let[n,a]of t){let o=document.getElementById(n);o&&Re(a(),o)}};document.readyState==="loading"?document.addEventListener("DOMContentLoaded",e):e()}it([["island-net-arming",()=>u(_e,{id:"net-arming-block",layout:pe,text:he})]]);})();
