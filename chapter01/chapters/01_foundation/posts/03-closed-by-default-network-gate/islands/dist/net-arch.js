(()=>{var pe={viewH:912,outlines:[{x:200,y:16,w:360,h:96,label:"Agent container \xB7 untrusted",variant:"danger"},{x:610,y:16,w:320,h:134,label:"Layer key"},{x:20,y:160,w:920,h:660,label:"Host \xB7 kernel and network namespace"},{x:40,y:400,w:880,h:230,label:"Bridge agents-net \xB7 172.30.0.0/24"}],nodes:{keyNetdev:{x:630,y:42,w:280,h:30,label:"Kernel netdev hooks (nftables)",variant:"solid-dark",small:!0},keyBridge:{x:630,y:78,w:280,h:30,label:"Bridge L2 port lock (iproute2)",variant:"solid-safe",small:!0},keyIptables:{x:630,y:114,w:280,h:30,label:"L3/L4 filtering (iptables)",small:!0},agent:{x:230,y:46,w:300,h:54,label:"Agent (gVisor guest)",sub:"guest eth0",variant:"solid-danger"},veth:{x:230,y:190,w:300,h:44,label:"Host veth",sub:"bridge port of the container"},port:{x:230,y:298,w:300,h:84,label:"Per-port table as_port_<id>",sub:["netdev nftables \xB7 policy drop","ingress + egress hooks on the veth","ingress accepts set mark 0x10000000"],variant:"solid-dark"},lock:{x:230,y:470,w:300,h:104,label:"Port controls and neighbour entry",sub:["veth: isolated on \xB7 learning off \xB7 locked on","veth: static FDB entry","bridge: permanent neighbour entry"],variant:"solid-safe"},gate:{x:620,y:470,w:280,h:104,label:"agents_gate (netdev)",sub:["policy drop \xB7 released = MAC + name","ingress: saddr in released + mark","then the gate clears the mark bit","egress: daddr in released"],variant:"solid-dark"},iptables:{x:590,y:680,w:340,h:104,label:"iptables DOCKER-USER / INPUT",sub:["rules keyed by container IP, tag agent-net:<name>","ACCEPT DNS 1.1.1.1 and 9.9.9.9 \xB7 model IP tcp/443","DROP all other traffic of the container IP","agent-net:gate: physdev DROP between ports"]},internet:{x:610,y:840,w:300,h:52,label:"Public internet",sub:"outside the host",variant:"solid-danger"}},links:[],ips:[{id:"IP-1",label:"1",points:[[380,100],[380,190]],dir:"end",at:[380,130]},{id:"IP-2",label:"2",points:[[380,234],[380,298]],dir:"end",at:[380,266]},{id:"IP-3",label:"3",points:[[380,382],[380,470]],dir:"end",at:[380,436]},{id:"IP-4",label:"4",points:[[530,522],[620,522]],dir:"end",at:[575,522]},{id:"IP-5",label:"5",points:[[760,574],[760,680]],dir:"end",at:[760,655]},{id:"IP-6",label:"6",points:[[760,784],[760,840]],dir:"end",at:[760,815]}],assets:[],threats:[]},he={label:"Block diagram of the network isolation architecture, top to bottom: the agent container, the host boundary and veth, the per-port netdev table, the agents-net bridge with its port controls, neighbour entry and agents_gate table, the iptables rules in DOCKER-USER and INPUT, and the public internet",hint:"Numbered circles mark each stage of the path from the agent to the public internet; hover or click one to see what the stage is configured to do. Box colour shows the layer: kernel netdev hooks (dark), bridge L2 port lock (green), iptables L3/L4 filtering (white).",items:{"IP-1":{name:"Host boundary: guest eth0 to host veth",desc:["\u2022 Where: the agent runs in a gVisor container whose only network interface is the guest eth0.","\u2022 Link: eth0 arrives on a veth on the host.","\u2022 Effect: every control in this diagram is applied on the host side of that veth, so it is intended to hold regardless of what runs inside the guest."].join(`
`),links:[]},"IP-2":{name:"Per-port table as_port_<id> (kernel netdev hooks)",desc:["\u2022 Table: a netdev nftables table with ingress and egress hooks bound to this container\u2019s veth.","\u2022 Initial state: loaded with policy drop and no accept rule, so it is intended to drop every frame on that port.","\u2022 Release: the release step adds accept rules for the agent\u2019s own MAC and IP.","\u2022 Mark: the ingress accept rules also set the mark bit 0x10000000 on every frame they accept.","\u2022 Gate dependency: the gate on the bridge requires that bit, so only a live port table can open the way.","\u2022 Restart: a restarted container gets a new veth with no port table, its frames carry no mark, and the gate drops them."].join(`
`),code:"agent-net: s10_close_port.go (ClosePort)",links:[]},"IP-3":{name:"Port controls on the veth, neighbour entry on the bridge (iproute2)",desc:["\u2022 On the veth: isolated on, learning off and locked on, plus a static FDB entry for the agent\u2019s MAC.","\u2022 On the bridge: a permanent neighbour entry that binds the agent\u2019s IP to that MAC.","\u2022 Purpose: these settings are configured to keep the agent from reaching other bridge ports and from taking over another address."].join(`
`),code:"agent-net: s13_apply_controls.go (ApplyControls)",links:[]},"IP-4":{name:"agents_gate (kernel netdev hooks on the bridge)",desc:["\u2022 Table: a netdev nftables table bound to the bridge device with policy drop.","\u2022 Ingress (from the container): passes only a frame whose source MAC is in the set released and that carries the mark bit from its own port table.","\u2022 Mark: the gate then clears the bit, so it never reaches host routing or iptables.","\u2022 Egress (towards the container): passes by destination MAC in released.","\u2022 Elements: each element of released carries the container name as its comment.","\u2022 Closed by default: a container is intended to stay closed from the moment it is attached until it is armed.","\u2022 Restart: an unmarked frame from a restarted container is dropped even if its MAC is still in the set."].join(`
`),code:"agent-net: s36_install_gate.go",links:[]},"IP-5":{name:"iptables DOCKER-USER and INPUT (L3/L4)",desc:["\u2022 Keying: rules are keyed by the container IP and tagged agent-net:<name>.","\u2022 Accept: its IP towards the DNS resolvers 1.1.1.1 and 9.9.9.9 (port 53) and towards the model API addresses on TCP 443.","\u2022 Drop: the rest of its traffic leaving the bridge.","\u2022 INPUT: configured to accept only replies to host-initiated flows and to drop other packets from the container IP.","\u2022 Bridged traffic: a separate rule tagged agent-net:gate drops it between ports (physdev)."].join(`
`),code:"agent-net: s13_apply_controls.go (iptPlan)",links:[]},"IP-6":{name:"Public internet",desc:["\u2022 Reachable: only the allowed DNS resolvers and the model API addresses are intended to be reachable.","\u2022 Timing: only after the port has been released.","\u2022 Everything else: traffic to any other destination is configured to be dropped by the rules above."].join(`
`),links:[]}}};var V,m,ke,st,$,be,ye,xe,ee,U,M,we,oe,te,ne,Pe,F={},j=[],lt=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,K=Array.isArray;function C(t,e){for(var n in e)t[n]=e[n];return t}function ae(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function ct(t,e,n){var r,a,o,d={};for(o in e)o=="key"?r=e[o]:o=="ref"?a=e[o]:d[o]=e[o];if(arguments.length>2&&(d.children=arguments.length>3?V.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(o in t.defaultProps)d[o]===void 0&&(d[o]=t.defaultProps[o]);return W(t,d,r,a,null)}function W(t,e,n,r,a){var o={type:t,props:e,key:n,ref:r,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:a==null?++ke:a,__i:-1,__u:0};return a==null&&m.vnode!=null&&m.vnode(o),o}function S(t){return t.children}function z(t,e){this.props=t,this.context=e}function T(t,e){if(e==null)return t.__?T(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?T(t):null}function gt(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,r=[],a=[],o=C({},e);o.__v=e.__v+1,m.vnode&&m.vnode(o),ie(t.__P,o,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,r,n==null?T(e):n,!!(32&e.__u),a),o.__v=e.__v,o.__.__k[o.__i]=o,$e(r,o,a),e.__e=e.__=null,o.__e!=n&&Se(o)}}function Se(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),Se(t)}function re(t){(!t.__d&&(t.__d=!0)&&$.push(t)&&!q.__r++||be!=m.debounceRendering)&&((be=m.debounceRendering)||ye)(q)}function q(){try{for(var t,e=1;$.length;)$.length>e&&$.sort(xe),t=$.shift(),e=$.length,gt(t)}finally{$.length=q.__r=0}}function Ie(t,e,n,r,a,o,d,l,g,c,u){var p,i,f,v,k,s,h=r&&r.__k||j,b=e.length;for(g=_t(n,e,h,g,b),p=0;p<b;p++)(f=n.__k[p])!=null&&(i=f.__i!=-1&&h[f.__i]||F,f.__i=p,s=ie(t,f,i,a,o,d,l,g,c,u),v=f.__e,f.ref&&i.ref!=f.ref&&(i.ref&&de(i.ref,null,f),u.push(f.ref,f.__c||v,f)),k==null&&v!=null&&(k=v),4&f.__u?(g=Ce(f,g,t),i.__e&&(i.__e=null)):typeof f.type=="function"&&s!==void 0?g=s:v&&(g=v.nextSibling),f.__u&=-7);return n.__e=k,g}function _t(t,e,n,r,a){var o,d,l,g,c,u=n.length,p=u,i=0;for(t.__k=new Array(a),o=0;o<a;o++)(d=e[o])!=null&&typeof d!="boolean"&&typeof d!="function"?(typeof d=="string"||typeof d=="number"||typeof d=="bigint"||d.constructor==String?d=t.__k[o]=W(null,d,null,null,null):K(d)?d=t.__k[o]=W(S,{children:d},null,null,null):d.constructor===void 0&&d.__b>0?d=t.__k[o]=W(d.type,d.props,d.key,d.ref?d.ref:null,d.__v):t.__k[o]=d,g=o+i,d.__=t,d.__b=t.__b+1,l=null,(c=d.__i=ft(d,n,g,p))!=-1&&(p--,(l=n[c])&&(l.__u|=2)),l==null||l.__v==null?(c==-1&&(a>u?i--:a<u&&i++),typeof d.type!="function"&&(d.__u|=4)):c!=g&&(c==g-1?i--:c==g+1?i++:(c>g?i--:i++,d.__u|=4))):t.__k[o]=null;if(p)for(o=0;o<u;o++)(l=n[o])!=null&&!(2&l.__u)&&(l.__e==r&&(r=T(l)),Te(l,l));return r}function Ce(t,e,n){var r,a;if(typeof t.type=="function"){for(r=t.__k,a=0;r&&a<r.length;a++)r[a]&&(r[a].__=t,e=Ce(r[a],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=T(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function ft(t,e,n,r){var a,o,d,l=t.key,g=t.type,c=e[n],u=c!=null&&(2&c.__u)==0;if(c===null&&l==null||u&&l==c.key&&g==c.type)return n;if(r>(u?1:0)){for(a=n-1,o=n+1;a>=0||o<e.length;)if((c=e[d=a>=0?a--:o++])!=null&&!(2&c.__u)&&l==c.key&&g==c.type)return d}return-1}function me(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||lt.test(e)?n:n+"px"}function N(t,e,n,r,a){var o,d;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof r=="string"&&(t.style.cssText=r=""),r)for(e in r)n&&e in n||me(t.style,e,"");if(n)for(e in n)r&&n[e]==r[e]||me(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")o=e!=(e=e.replace(we,"$1")),d=e.toLowerCase(),e=d in t||e=="onFocusOut"||e=="onFocusIn"?d.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+o]=n,n?r?n[M]=r[M]:(n[M]=oe,t.addEventListener(e,o?ne:te,o)):t.removeEventListener(e,o?ne:te,o);else{if(a=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function ve(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[U]==null)e[U]=oe++;else if(e[U]<n[M])return;return n(m.event?m.event(e):e)}}}function ie(t,e,n,r,a,o,d,l,g,c){var u,p,i,f,v,k,s,h,b,P,E,D,R,ue,O,Z,I=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(g=!!(32&n.__u),o=[l=e.__e=n.__e]),(u=m.__b)&&u(e);e:if(typeof I=="function"){p=d.length;try{if(b=e.props,P=I.prototype&&I.prototype.render,E=(u=I.contextType)&&r[u.__c],D=u?E?E.props.value:u.__:r,n.__c?h=(i=e.__c=n.__c).__=i.__E:(P?e.__c=i=new I(b,D):(e.__c=i=new z(b,D),i.constructor=I,i.render=pt),E&&E.sub(i),i.state||(i.state={}),i.__n=r,f=i.__d=!0,i.__h=[],i._sb=[]),P&&i.__s==null&&(i.__s=i.state),P&&I.getDerivedStateFromProps!=null&&(i.__s==i.state&&(i.__s=C({},i.__s)),C(i.__s,I.getDerivedStateFromProps(b,i.__s))),v=i.props,k=i.state,i.__v=e,f)P&&I.getDerivedStateFromProps==null&&i.componentWillMount!=null&&i.componentWillMount(),P&&i.componentDidMount!=null&&i.__h.push(i.componentDidMount);else{if(P&&I.getDerivedStateFromProps==null&&b!==v&&i.componentWillReceiveProps!=null&&i.componentWillReceiveProps(b,D),e.__v==n.__v||!i.__e&&i.shouldComponentUpdate!=null&&i.shouldComponentUpdate(b,i.__s,D)===!1){e.__v!=n.__v&&(i.props=b,i.state=i.__s,i.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(A){A&&(A.__=e)}),j.push.apply(i.__h,i._sb),i._sb=[],i.__h.length&&d.push(i),l=T(n);break e}i.componentWillUpdate!=null&&i.componentWillUpdate(b,i.__s,D),P&&i.componentDidUpdate!=null&&i.__h.push(function(){i.componentDidUpdate(v,k,s)})}if(i.context=D,i.props=b,i.__P=t,i.__e=!1,R=m.__r,ue=0,P)i.state=i.__s,i.__d=!1,R&&R(e),u=i.render(i.props,i.state,i.context),j.push.apply(i.__h,i._sb),i._sb=[];else do i.__d=!1,R&&R(e),u=i.render(i.props,i.state,i.context),i.state=i.__s;while(i.__d&&++ue<25);i.state=i.__s,i.getChildContext!=null&&(r=C(C({},r),i.getChildContext())),P&&!f&&i.getSnapshotBeforeUpdate!=null&&(s=i.getSnapshotBeforeUpdate(v,k)),O=u!=null&&u.type===S&&u.key==null?De(u.props.children):u,l=Ie(t,K(O)?O:[O],e,n,r,a,o,d,l,g,c),i.base=e.__e,e.__u&=-161,i.__h.length&&d.push(i),h&&(i.__E=i.__=null)}catch(A){if(d.length=p,e.__v=null,g||o!=null){if(A.then){for(e.__u|=g?160:128;l&&l.nodeType==8&&l.nextSibling;)l=l.nextSibling;o!=null&&(o[o.indexOf(l)]=null),e.__e=l}else if(o!=null)for(Z=o.length;Z--;)ae(o[Z])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),A.then||Ee(e),m.__e(A,e,n)}}else o==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):l=e.__e=ut(n.__e,e,n,r,a,o,d,g,c);return(u=m.diffed)&&u(e),128&e.__u?void 0:l}function Ee(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Ee))}function $e(t,e,n){for(var r=0;r<n.length;r++)de(n[r],n[++r],n[++r]);m.__c&&m.__c(e,t),t.some(function(a){try{t=a.__h,a.__h=[],t.some(function(o){o.call(a)})}catch(o){m.__e(o,a.__v)}})}function De(t){return typeof t!="object"||t==null||t.__b>0?t:K(t)?t.map(De):t.constructor!==void 0?null:C({},t)}function ut(t,e,n,r,a,o,d,l,g){var c,u,p,i,f,v,k,s=n.props||F,h=e.props,b=e.type;if(b=="svg"?a="http://www.w3.org/2000/svg":b=="math"?a="http://www.w3.org/1998/Math/MathML":a||(a="http://www.w3.org/1999/xhtml"),o!=null){for(c=0;c<o.length;c++)if((f=o[c])&&"setAttribute"in f==!!b&&(b?f.localName==b:f.nodeType==3)){t=f,o[c]=null;break}}if(t==null){if(b==null)return document.createTextNode(h);t=document.createElementNS(a,b,h.is&&h),l&&(m.__m&&m.__m(e,o),l=!1),o=null}if(b==null)s===h||l&&t.data==h||(t.data=h);else{if(o=b=="textarea"&&h.defaultValue!=null?null:o&&V.call(t.childNodes),!l&&o!=null)for(s={},c=0;c<t.attributes.length;c++)s[(f=t.attributes[c]).name]=f.value;for(c in s)f=s[c],c=="dangerouslySetInnerHTML"?p=f:c=="children"||c in h||c=="value"&&"defaultValue"in h||c=="checked"&&"defaultChecked"in h||N(t,c,null,f,a);for(c in h)f=h[c],c=="children"?i=f:c=="dangerouslySetInnerHTML"?u=f:c=="value"?v=f:c=="checked"?k=f:l&&typeof f!="function"||s[c]===f||N(t,c,f,s[c],a);if(u)l||p&&(u.__html==p.__html||u.__html==t.innerHTML)||(t.innerHTML=u.__html),e.__k=[];else if(p&&(t.innerHTML=""),Ie(e.type=="template"?t.content:t,K(i)?i:[i],e,n,r,b=="foreignObject"?"http://www.w3.org/1999/xhtml":a,o,d,o?o[0]:n.__k&&T(n,0),l,g),o!=null)for(c=o.length;c--;)ae(o[c]);l&&b!="textarea"||(c="value",b=="progress"&&v==null?t.removeAttribute("value"):v!=null&&(v!==t[c]||b=="progress"&&!v||b=="option"&&v!=s[c])&&N(t,c,v,s[c],a),c="checked",k!=null&&k!=t[c]&&N(t,c,k,s[c],a))}return t}function de(t,e,n){try{if(typeof t=="function"){var r=typeof t.__u=="function";r&&t.__u(),r&&e==null||(t.__u=t(e))}else t.current=e}catch(a){m.__e(a,n)}}function Te(t,e,n){var r,a;if(m.unmount&&m.unmount(t),(r=t.ref)&&(r.current&&r.current!=t.__e||de(r,null,e)),(r=t.__c)!=null){if(r.componentWillUnmount)try{r.componentWillUnmount()}catch(o){m.__e(o,e)}r.base=r.__P=r.__n=null}if(r=t.__k)for(a=0;a<r.length;a++)r[a]&&Te(r[a],e,n||typeof t.type!="function");n||ae(t.__e),t.__c=t.__=t.__e=void 0}function pt(t,e,n){return this.constructor(t,n)}function Ae(t,e,n){var r,a,o,d;e==document&&(e=document.documentElement),m.__&&m.__(t,e),a=(r=typeof n=="function")?null:n&&n.__k||e.__k,o=[],d=[],ie(e,t=(!r&&n||e).__k=ct(S,null,[t]),a||F,F,e.namespaceURI,!r&&n?[n]:a?null:e.firstChild?V.call(e.childNodes):null,o,!r&&n?n:a?a.__e:e.firstChild,r,d),$e(o,t,d),t.props.children=null}function He(t){function e(n){var r,a;return this.getChildContext||(r=new Set,(a={})[e.__c]=this,this.getChildContext=function(){return a},this.componentWillUnmount=function(){r=null},this.shouldComponentUpdate=function(o){this.props.value!=o.value&&r.forEach(function(d){d.__e=!0,re(d)})},this.sub=function(o){r.add(o);var d=o.componentWillUnmount;o.componentWillUnmount=function(){r&&r.delete(o),d&&d.call(o)}}),n.children}return e.__c="__cC"+Pe++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,r){return n.children(r)}).contextType=e,e}V=j.slice,m={__e:function(t,e,n,r){for(var a,o,d;e=e.__;)if((a=e.__c)&&!a.__)try{if((o=a.constructor)&&o.getDerivedStateFromError!=null&&(a.setState(o.getDerivedStateFromError(t)),d=a.__d),a.componentDidCatch!=null&&(a.componentDidCatch(t,r||{}),d=a.__d),d)return a.__E=a}catch(l){t=l}throw t}},ke=0,st=function(t){return t!=null&&t.constructor===void 0},z.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=C({},this.state),typeof t=="function"&&(t=t(C({},n),this.props)),t&&C(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),re(this))},z.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),re(this))},z.prototype.render=S,$=[],ye=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,xe=function(t,e){return t.__v.__b-e.__v.__b},q.__r=0,ee=Math.random().toString(8),U="__d"+ee,M="__a"+ee,we=/(PointerCapture)$|Capture$/i,oe=0,te=ve(!1),ne=ve(!0),Pe=0;var H,y,se,Re,Y=0,ze=[],x=m,Me=x.__b,Le=x.__r,Be=x.diffed,Oe=x.__c,Ne=x.unmount,Ue=x.__;function X(t,e){x.__h&&x.__h(y,t,Y||e),Y=0;var n=y.__H||(y.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function L(t){return Y=1,ht(Ke,t)}function ht(t,e,n){var r=X(H++,2);if(r.t=t,!r.__c&&(r.__=[n?n(e):Ke(void 0,e),function(l){var g=r.__N?r.__N[0]:r.__[0],c=r.t(g,l);g!==c&&(r.__N=[c,r.__[1]],r.__c.setState({}))}],r.__c=y,!y.__f)){var a=function(l,g,c){if(!r.__c.__H)return!0;var u=!1,p=r.__c.props!==l;if(r.__c.__H.__.some(function(f){if(f.__N){u=!0;var v=f.__[0];f.__=f.__N,f.__N=void 0,v!==f.__[0]&&(p=!0)}}),o){var i=o.call(this,l,g,c);return u?i||p:i}return!u||p};y.__f=!0;var o=y.shouldComponentUpdate,d=y.componentWillUpdate;y.componentWillUpdate=function(l,g,c){if(this.__e){var u=o;o=void 0,a(l,g,c),o=u}d&&d.call(this,l,g,c)},y.shouldComponentUpdate=a}return r.__N||r.__}function Fe(t,e){var n=X(H++,4);!x.__s&&Ve(n.__H,e)&&(n.__=t,n.u=e,y.__h.push(n))}function je(t){return Y=5,bt(function(){return{current:t}},[])}function bt(t,e){var n=X(H++,7);return Ve(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function qe(t){var e=y.context[t.__c],n=X(H++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(y)),e.props.value):t.__}function mt(){for(var t;t=ze.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(G),e.__h.some(le),e.__h=[]}catch(n){e.__h=[],x.__e(n,t.__v)}}}x.__b=function(t){y=null,Me&&Me(t)},x.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),Ue&&Ue(t,e)},x.__r=function(t){Le&&Le(t),H=0;var e=(y=t.__c).__H;e&&(se===y?(e.__h=[],y.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(G),e.__h.some(le),e.__h=[],H=0)),se=y},x.diffed=function(t){Be&&Be(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(ze.push(e)!==1&&Re===x.requestAnimationFrame||((Re=x.requestAnimationFrame)||vt)(mt)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),se=y=null},x.__c=function(t,e){e.some(function(n){try{n.__h.some(G),n.__h=n.__h.filter(function(r){return!r.__||le(r)})}catch(r){e.some(function(a){a.__h&&(a.__h=[])}),e=[],x.__e(r,n.__v)}}),Oe&&Oe(t,e)},x.unmount=function(t){Ne&&Ne(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(r){try{G(r)}catch(a){e=a}}),n.__H=void 0,e&&x.__e(e,n.__v))};var We=typeof requestAnimationFrame=="function";function vt(t){var e,n=function(){clearTimeout(r),We&&cancelAnimationFrame(e),setTimeout(t)},r=setTimeout(n,35);We&&(e=requestAnimationFrame(n))}function G(t){var e=y,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),y=e}function le(t){var e=y;t.__c=t.__(),y=e}function Ve(t,e){return!t||t.length!==e.length||e.some(function(n,r){return n!==t[r]})}function Ke(t,e){return typeof e=="function"?e(t):e}var Ge="(prefers-color-scheme: dark)";function kt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function yt(t){let e=t;for(;e;){let n=kt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(Ge).matches?"dark":"light"}function Ye(t){let[e,n]=L("light");return Fe(()=>{let r=t.current;if(!r)return;let a=()=>n(yt(r.parentElement||r));a();let o=new MutationObserver(a),d={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};o.observe(document.documentElement,d),document.body&&o.observe(document.body,d);let l=window.matchMedia?window.matchMedia(Ge):null;return l&&l.addEventListener("change",a),()=>{o.disconnect(),l&&l.removeEventListener("change",a)}},[t]),e}var w={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",edgeStrokeBlocked:2.75,edgeDashBlocked:"0.1 6",blockMarkR:6,edgeHitWidth:14,badgeRadius:13,badgeRadiusActive:15,diamondExtra:3,squareRadius:3,badgeOnEdgeMin:60,fragStroke:1.25,fragDash:"4 3",fragTagW:34,fragTagH:20,fragTagCut:6},xt=`
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
  .dg-diagram { ${xt} }
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
`,Qe="dg-diagram-styles";function Je(){if(typeof document=="undefined"||document.getElementById(Qe))return;let t=document.createElement("style");t.id=Qe,t.textContent=wt,document.head.appendChild(t)}var Pt=0,Ft=Array.isArray;function _(t,e,n,r,a,o){e||(e={});var d,l,g=e;if("ref"in g)for(l in g={},e)l=="ref"?d=e[l]:g[l]=e[l];var c={type:t,props:g,key:n,ref:d,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--Pt,__i:-1,__u:0,__source:a,__self:o};if(typeof t=="function"&&(d=t.defaultProps))for(l in d)g[l]===void 0&&(g[l]=d[l]);return m.vnode&&m.vnode(c),c}var tt=He({markerPrefix:"dg-arrow"});function Q(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function St(t){let e=[],n=0;for(let a=1;a<t.length;a++){let[o,d]=t[a-1],[l,g]=t[a],c=Math.hypot(l-o,g-d);e.push(c),n+=c}let r=n/2;for(let a=0;a<e.length;a++){if(r<=e[a]||a===e.length-1){let[o,d]=t[a],[l,g]=t[a+1],c=e[a]===0?0:r/e[a];return[o+(l-o)*c,d+(g-d)*c]}r-=e[a]}return t[t.length-1]}function It(t,e="Edge"){for(let n=1;n<t.length;n++){let[r,a]=t[n-1],[o,d]=t[n];if(r!==o&&a!==d)throw new Error(`${e}: diagonal segment (${r},${a})\u2192(${o},${d}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function J({x:t,y:e,w:n,h:r,label:a,variant:o="neutral",filled:d}){return _("g",{class:`dg-outline dg-outline-${o}${d?" dg-outline-filled":""}`,children:[_("rect",{x:t,y:e,width:n,height:r,rx:w.outlineRadius,"stroke-width":w.outlineStroke,"stroke-dasharray":w.outlineDash}),a?_("text",{x:t+14,y:e+22,class:"dg-label dg-label-outline",children:a}):null]})}function ce({x:t,y:e,w:n,h:r,label:a,sub:o,variant:d="default",small:l}){let g=d.startsWith("solid-"),c=o?[].concat(o):[],u=l?14:18,p=c.length?e+r/2-6-(c.length-1)*14/2:e+r/2+5;return _("g",{class:`dg-box dg-box-${d}${g?" dg-box-solid":""}`,children:[_("rect",{x:t,y:e,width:n,height:r,rx:w.boxRadius,"stroke-width":w.boxStroke}),_("text",{x:t+n/2,y:p,"text-anchor":"middle",class:l?"dg-label dg-label-sm":"dg-label dg-label-bold",children:a}),c.map((i,f)=>_("text",{x:t+n/2,y:p+u+f*14,"text-anchor":"middle",class:"dg-label dg-label-sub",children:i},f))]})}function ge({points:t,dashed:e,blocked:n,active:r,dir:a,id:o,onSelect:d,onHover:l}){It(t,o?`Edge ${o}`:"Edge");let{markerPrefix:g}=qe(tt),c=`${g}-${r?"active":"default"}`,u=a&&a!=="none",p=typeof d=="function",i=(s,h)=>{s.pointerType==="mouse"&&l&&l(h)},f=p?{role:"button",tabindex:0,"aria-label":o,onClick:()=>d(o),onPointerEnter:s=>i(s,o),onPointerLeave:s=>i(s,null),onKeyDown:s=>{(s.key==="Enter"||s.key===" ")&&(s.preventDefault(),d(o))}}:{},v=p?_("polyline",{points:Q(t),fill:"none",stroke:"transparent","pointer-events":"stroke","stroke-width":w.edgeHitWidth}):null;if(n){let[s,h]=St(t),b=w.blockMarkR,P=`dg-marker-blocked${r?" dg-marker-blocked-active":""}`,E=_(S,{children:[_("polyline",{class:`dg-edge dg-edge-blocked${r?" dg-edge-active":""}`,points:Q(t),fill:"none","stroke-width":w.edgeStrokeBlocked,"stroke-dasharray":w.edgeDashBlocked}),_("path",{d:`M${s-b},${h-b} L${s+b},${h+b} M${s+b},${h-b} L${s-b},${h+b}`,class:P,fill:"none","stroke-width":w.edgeStrokeBlocked,"stroke-linecap":"round"})]});return p?_("g",{class:"dg-edge-hit",...f,children:[v,E]}):E}let k=_("polyline",{class:`dg-edge${r?" dg-edge-active":""}`,points:Q(t),fill:"none","stroke-width":r?w.edgeStrokeActive:w.edgeStroke,"stroke-dasharray":e?w.edgeDash:void 0,"marker-end":u?`url(#${c})`:void 0,"marker-start":a==="both"?`url(#${c})`:void 0});return p?_("g",{class:"dg-edge-hit",...f,children:[v,k]}):k}function nt({id:t,label:e,name:n,x:r,y:a,active:o,related:d,pressed:l,shape:g="circle",onSelect:c,onHover:u}){let p=(k,s)=>{k.pointerType==="mouse"&&u&&u(s)},i=o?w.badgeRadiusActive:w.badgeRadius,f=i+w.diamondExtra,v=`dg-badge dg-badge-${g}${o?" dg-badge-active":d?" dg-badge-related":""}`;return _("g",{class:v,role:"button",tabindex:"0","aria-pressed":l,"aria-label":n?`${t}: ${n}`:t,onClick:()=>c(t),onPointerEnter:k=>p(k,t),onPointerLeave:k=>p(k,null),onKeyDown:k=>{(k.key==="Enter"||k.key===" ")&&(k.preventDefault(),c(t))},children:[g==="diamond"?_("polygon",{points:Q([[r,a-f],[r+f,a],[r,a+f],[r-f,a]]),"stroke-width":"1.5"}):g==="square"?_("rect",{x:r-i,y:a-i,width:2*i,height:2*i,rx:w.squareRadius,"stroke-width":"1.5"}):_("circle",{cx:r,cy:a,r:i,"stroke-width":"1.5"}),_("text",{x:r,y:a+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function rt(){let[t,e]=L(null),[n,r]=L(null);return{shownId:n||t,pinnedId:t,select:o=>e(d=>d===o?null:o),setHover:r}}function Ct({tone:t="neutral",id:e,text:n}){return _("span",{class:`dg-chip dg-chip-${t}`,children:[_("span",{class:"dg-chip-id",children:e}),n?_("span",{class:"dg-chip-desc",children:n}):null]})}function ot({id:t,title:e,desc:n,chips:r,hint:a,children:o}){return _("div",{class:"dg-panel","aria-live":"polite",children:e?_(S,{children:[_("div",{class:"dg-panel-title",children:[t?_(S,{children:[t," \u2014 "]}):null,e]}),n?_("div",{class:"dg-panel-desc",children:n}):null,r&&r.length?_("div",{class:"dg-chips",children:r.map(d=>_(Ct,{...d},`${d.tone}-${d.id}`))}):null,o]}):_("div",{class:"dg-hint",children:a})})}function et({id:t,active:e}){return _("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:_("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}var Et=680,$t=380;function at({id:t,viewH:e,viewW:n=960,label:r,footer:a,scrollHint:o,fluid:d,children:l}){Je();let g=je(null),c=Ye(g),u=`dg-arrow-${t}`,p=n/960,i=p!==1&&!d?{flexBasis:`${Math.round(Et*p)}px`,minWidth:`${Math.round($t*p)}px`}:void 0,f=_("svg",{viewBox:`0 0 ${n} ${e}`,style:o?void 0:i,role:"group","aria-label":r,children:[_("defs",{children:[_(et,{id:`${u}-default`}),_(et,{id:`${u}-active`,active:!0})]}),l]});return _("div",{class:d?"dg-diagram dg-fluid":"dg-diagram","data-dg-theme":c,ref:g,children:[_(tt.Provider,{value:{markerPrefix:u},children:o?_("div",{class:"dg-scrollwrap",style:i,children:[_("div",{class:"dg-scroll",children:f}),_("div",{class:"dg-scroll-hint",children:o})]}):f}),a]})}var B={"IP-1":"Host boundary","IP-2":"Per-port table","IP-3":"Port controls","IP-4":"agents_gate","IP-5":"iptables rules","IP-6":"Public internet","IP-S1":"Validate invocation","IP-S2":"Privilege and engine","IP-S3":"Record engine state","IP-S4":"Network and gate","IP-S5":"Require paused","IP-S6":"Clean slate","IP-S7":"Identify veth","IP-S8":"Close port","IP-S9":"Apply controls","IP-S10":"Verify controls","IP-S11":"Release port","IP-S12":"Verify engine state","IP-S13":"Report armed"};var Dt={ip:"circle",asset:"diamond",threat:"square"},Tt={ip:"neutral",asset:"info",threat:"danger"},At={safe:"solid-safe",info:"solid-info",danger:"solid-danger",neutral:"default"};function _e(t){return t.startsWith("IP-")?"ip":t.startsWith("AS-")?"asset":"threat"}function Ht(t,e){var r;let n=new Set(((r=t[e])==null?void 0:r.links)||[]);for(let[a,o]of Object.entries(t))(o.links||[]).includes(e)&&n.add(a);return n.delete(e),[...n]}function Rt(t){let e=t.status||{},n=l=>{let g=e[l];return g==null?null:typeof g=="string"?{status:g}:g},r=l=>{var g;return(g=n(l))==null?void 0:g.status},a=l=>(t.statusTone||{})[l]||"neutral",o=l=>(t.statusLabel||{})[l];return{entry:n,key:r,tone:a,label:o,chip:l=>(t.statusChip||{})[l]||o(l)}}function Mt({geom:t,title:e,st:n,keys:r}){let a=t.order||r;return _(S,{children:[e?_("text",{x:t.x,y:t.y+16,class:"dg-label dg-label-sub",children:e}):null,a.map((o,d)=>{let l=t.x+(t.titleW||0)+d*t.step;return _("g",{children:[_(J,{x:l,y:t.y,w:t.swatch,h:t.swatch,variant:n.tone(o),filled:!0}),_("text",{x:l+t.swatch+8,y:t.y+16,class:"dg-label dg-label-sub",children:n.label(o)})]},`legend-${o}`)})]})}function fe({id:t,layout:e,text:n}){let{shownId:r,pinnedId:a,select:o,setHover:d}=rt(),l=r?n.items[r]:null,g=r?Ht(n.items,r):[],c=Rt(n),u=s=>{let h=c.key(s);return h?`${B[s]} \xB7 ${c.chip(h)}`:B[s]},p=r?c.entry(r):null,i=l?(l.reqs||[]).filter(s=>{var h;return(h=c.entry(s))==null?void 0:h.note}):[],f=["ip","asset","threat"],v=l?[...p?[{tone:c.tone(p.status),id:n.statusChipId||"Status",text:c.chip(p.status)}]:[],...g.slice().sort((s,h)=>f.indexOf(_e(s))-f.indexOf(_e(h))).map(s=>{var h;return{tone:Tt[_e(s)],id:s,text:B[s]||((h=n.items[s])==null?void 0:h.name)}}),...(l.goals||[]).map(s=>({tone:"neutral",id:s,text:B[s]})),...(l.reqs||[]).map(s=>({tone:"safe",id:s,text:u(s)}))]:[],k=[...e.ips.map(s=>({...s,kind:"ip"})),...e.assets.map(s=>({...s,kind:"asset"})),...e.threats.map(s=>({...s,kind:"threat"}))];return _(at,{id:t,viewH:e.viewH,viewW:e.viewW,label:n.label,footer:_(ot,{id:r,title:l==null?void 0:l.name,desc:l==null?void 0:l.desc,chips:v,hint:n.hint||"Circles are interaction points, diamonds assets, squares threat cases. Hover or click one to see what it connects to; the panel lists them with the capability goals (grey) and requirements (green) that hold there.",children:[l!=null&&l.code?_("div",{class:"dg-panel-desc",children:["Code: ",l.code]}):null,p!=null&&p.note?_("div",{class:"dg-panel-desc",children:p.note}):null,i.map(s=>_("div",{class:"dg-panel-desc",children:[_("strong",{children:s})," \u2014 ",c.entry(s).note]},`note-${s}`))]}),children:[e.outlines.map(s=>_(J,{...s},s.label)),Object.entries(e.nodes).map(([s,h])=>_(ce,{...h},s)),(e.statusBoxes||[]).map(s=>_(ce,{x:s.x,y:s.y,w:s.w,h:s.h,small:s.small,label:s.id,sub:c.label(c.key(s.id)),variant:At[c.tone(c.key(s.id))]},`status-${s.id}`)),e.links.map((s,h)=>_(ge,{id:s.flow||`${t}-link-${h}`,points:s.points,dir:s.dir,dashed:s.dashed,blocked:s.blocked,active:s.flow?r===s.flow:void 0,onSelect:s.flow?o:void 0,onHover:s.flow?d:void 0},`l${h}`)),e.ips.map(s=>_(ge,{id:`${t}-${s.id}`,points:s.points,dir:s.dir,dashed:s.dashed,active:r===s.id},s.id)),e.statusHalo?k.filter(s=>c.key(s.id)).map(s=>_(J,{x:s.at[0]-e.statusHalo,y:s.at[1]-e.statusHalo,w:2*e.statusHalo,h:2*e.statusHalo,variant:c.tone(c.key(s.id)),filled:!0},`halo-${s.id}`)):null,e.statusLegend?_(Mt,{geom:e.statusLegend,title:n.statusLegend,st:c,keys:Object.keys(n.statusLabel||{})}):null,k.map(s=>{var h;return _(nt,{id:s.id,label:s.label,name:(h=n.items[s.id])==null?void 0:h.name,shape:Dt[s.kind],x:s.at[0],y:s.at[1],active:r===s.id,pressed:a===s.id,onSelect:o,onHover:d},s.id)})]})}function it(t){let e=()=>{for(let[n,r]of t){let a=document.getElementById(n);a&&Ae(r(),a)}};document.readyState==="loading"?document.addEventListener("DOMContentLoaded",e):e()}it([["island-net-arch",()=>_(fe,{id:"net-arch-block",layout:pe,text:he})]]);})();
