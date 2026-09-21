(()=>{var G,g,me,it,D,ge,be,ve,Z,U,L,ye,re,ee,te,xe,z={},q=[],st=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,K=Array.isArray;function P(t,e){for(var n in e)t[n]=e[n];return t}function oe(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function dt(t,e,n){var o,a,r,s={};for(r in e)r=="key"?o=e[r]:r=="ref"?a=e[r]:s[r]=e[r];if(arguments.length>2&&(s.children=arguments.length>3?G.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(r in t.defaultProps)s[r]===void 0&&(s[r]=t.defaultProps[r]);return W(t,s,o,a,null)}function W(t,e,n,o,a){var r={type:t,props:e,key:n,ref:o,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:a==null?++me:a,__i:-1,__u:0};return a==null&&g.vnode!=null&&g.vnode(r),r}function A(t){return t.children}function j(t,e){this.props=t,this.context=e}function O(t,e){if(e==null)return t.__?O(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?O(t):null}function lt(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,o=[],a=[],r=P({},e);r.__v=e.__v+1,g.vnode&&g.vnode(r),ae(t.__P,r,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,o,n==null?O(e):n,!!(32&e.__u),a),r.__v=e.__v,r.__.__k[r.__i]=r,Ce(o,r,a),e.__e=e.__=null,r.__e!=n&&ke(r)}}function ke(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),ke(t)}function ne(t){(!t.__d&&(t.__d=!0)&&D.push(t)&&!V.__r++||ge!=g.debounceRendering)&&((ge=g.debounceRendering)||be)(V)}function V(){try{for(var t,e=1;D.length;)D.length>e&&D.sort(ve),t=D.shift(),e=D.length,lt(t)}finally{D.length=V.__r=0}}function we(t,e,n,o,a,r,s,d,_,l,u){var h,i,p,b,k,x,y=o&&o.__k||q,f=e.length;for(_=ct(n,e,y,_,f),h=0;h<f;h++)(p=n.__k[h])!=null&&(i=p.__i!=-1&&y[p.__i]||z,p.__i=h,x=ae(t,p,i,a,r,s,d,_,l,u),b=p.__e,p.ref&&i.ref!=p.ref&&(i.ref&&ie(i.ref,null,p),u.push(p.ref,p.__c||b,p)),k==null&&b!=null&&(k=b),4&p.__u?(_=Se(p,_,t),i.__e&&(i.__e=null)):typeof p.type=="function"&&x!==void 0?_=x:b&&(_=b.nextSibling),p.__u&=-7);return n.__e=k,_}function ct(t,e,n,o,a){var r,s,d,_,l,u=n.length,h=u,i=0;for(t.__k=new Array(a),r=0;r<a;r++)(s=e[r])!=null&&typeof s!="boolean"&&typeof s!="function"?(typeof s=="string"||typeof s=="number"||typeof s=="bigint"||s.constructor==String?s=t.__k[r]=W(null,s,null,null,null):K(s)?s=t.__k[r]=W(A,{children:s},null,null,null):s.constructor===void 0&&s.__b>0?s=t.__k[r]=W(s.type,s.props,s.key,s.ref?s.ref:null,s.__v):t.__k[r]=s,_=r+i,s.__=t,s.__b=t.__b+1,d=null,(l=s.__i=_t(s,n,_,h))!=-1&&(h--,(d=n[l])&&(d.__u|=2)),d==null||d.__v==null?(l==-1&&(a>u?i--:a<u&&i++),typeof s.type!="function"&&(s.__u|=4)):l!=_&&(l==_-1?i--:l==_+1?i++:(l>_?i--:i++,s.__u|=4))):t.__k[r]=null;if(h)for(r=0;r<u;r++)(d=n[r])!=null&&!(2&d.__u)&&(d.__e==o&&(o=O(d)),Pe(d,d));return o}function Se(t,e,n){var o,a;if(typeof t.type=="function"){for(o=t.__k,a=0;o&&a<o.length;a++)o[a]&&(o[a].__=t,e=Se(o[a],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=O(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function _t(t,e,n,o){var a,r,s,d=t.key,_=t.type,l=e[n],u=l!=null&&(2&l.__u)==0;if(l===null&&d==null||u&&d==l.key&&_==l.type)return n;if(o>(u?1:0)){for(a=n-1,r=n+1;a>=0||r<e.length;)if((l=e[s=a>=0?a--:r++])!=null&&!(2&l.__u)&&d==l.key&&_==l.type)return s}return-1}function fe(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||st.test(e)?n:n+"px"}function F(t,e,n,o,a){var r,s;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof o=="string"&&(t.style.cssText=o=""),o)for(e in o)n&&e in n||fe(t.style,e,"");if(n)for(e in n)o&&n[e]==o[e]||fe(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")r=e!=(e=e.replace(ye,"$1")),s=e.toLowerCase(),e=s in t||e=="onFocusOut"||e=="onFocusIn"?s.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+r]=n,n?o?n[L]=o[L]:(n[L]=re,t.addEventListener(e,r?te:ee,r)):t.removeEventListener(e,r?te:ee,r);else{if(a=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function he(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[U]==null)e[U]=re++;else if(e[U]<n[L])return;return n(g.event?g.event(e):e)}}}function ae(t,e,n,o,a,r,s,d,_,l){var u,h,i,p,b,k,x,y,f,C,$,I,B,pe,R,X,T=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(_=!!(32&n.__u),r=[d=e.__e=n.__e]),(u=g.__b)&&u(e);e:if(typeof T=="function"){h=s.length;try{if(f=e.props,C=T.prototype&&T.prototype.render,$=(u=T.contextType)&&o[u.__c],I=u?$?$.props.value:u.__:o,n.__c?y=(i=e.__c=n.__c).__=i.__E:(C?e.__c=i=new T(f,I):(e.__c=i=new j(f,I),i.constructor=T,i.render=pt),$&&$.sub(i),i.state||(i.state={}),i.__n=o,p=i.__d=!0,i.__h=[],i._sb=[]),C&&i.__s==null&&(i.__s=i.state),C&&T.getDerivedStateFromProps!=null&&(i.__s==i.state&&(i.__s=P({},i.__s)),P(i.__s,T.getDerivedStateFromProps(f,i.__s))),b=i.props,k=i.state,i.__v=e,p)C&&T.getDerivedStateFromProps==null&&i.componentWillMount!=null&&i.componentWillMount(),C&&i.componentDidMount!=null&&i.__h.push(i.componentDidMount);else{if(C&&T.getDerivedStateFromProps==null&&f!==b&&i.componentWillReceiveProps!=null&&i.componentWillReceiveProps(f,I),e.__v==n.__v||!i.__e&&i.shouldComponentUpdate!=null&&i.shouldComponentUpdate(f,i.__s,I)===!1){e.__v!=n.__v&&(i.props=f,i.state=i.__s,i.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(N){N&&(N.__=e)}),q.push.apply(i.__h,i._sb),i._sb=[],i.__h.length&&s.push(i),d=O(n);break e}i.componentWillUpdate!=null&&i.componentWillUpdate(f,i.__s,I),C&&i.componentDidUpdate!=null&&i.__h.push(function(){i.componentDidUpdate(b,k,x)})}if(i.context=I,i.props=f,i.__P=t,i.__e=!1,B=g.__r,pe=0,C)i.state=i.__s,i.__d=!1,B&&B(e),u=i.render(i.props,i.state,i.context),q.push.apply(i.__h,i._sb),i._sb=[];else do i.__d=!1,B&&B(e),u=i.render(i.props,i.state,i.context),i.state=i.__s;while(i.__d&&++pe<25);i.state=i.__s,i.getChildContext!=null&&(o=P(P({},o),i.getChildContext())),C&&!p&&i.getSnapshotBeforeUpdate!=null&&(x=i.getSnapshotBeforeUpdate(b,k)),R=u!=null&&u.type===A&&u.key==null?Te(u.props.children):u,d=we(t,K(R)?R:[R],e,n,o,a,r,s,d,_,l),i.base=e.__e,e.__u&=-161,i.__h.length&&s.push(i),y&&(i.__E=i.__=null)}catch(N){if(s.length=h,e.__v=null,_||r!=null){if(N.then){for(e.__u|=_?160:128;d&&d.nodeType==8&&d.nextSibling;)d=d.nextSibling;r!=null&&(r[r.indexOf(d)]=null),e.__e=d}else if(r!=null)for(X=r.length;X--;)oe(r[X])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),N.then||Ee(e),g.__e(N,e,n)}}else r==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):d=e.__e=ut(n.__e,e,n,o,a,r,s,_,l);return(u=g.diffed)&&u(e),128&e.__u?void 0:d}function Ee(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Ee))}function Ce(t,e,n){for(var o=0;o<n.length;o++)ie(n[o],n[++o],n[++o]);g.__c&&g.__c(e,t),t.some(function(a){try{t=a.__h,a.__h=[],t.some(function(r){r.call(a)})}catch(r){g.__e(r,a.__v)}})}function Te(t){return typeof t!="object"||t==null||t.__b>0?t:K(t)?t.map(Te):t.constructor!==void 0?null:P({},t)}function ut(t,e,n,o,a,r,s,d,_){var l,u,h,i,p,b,k,x=n.props||z,y=e.props,f=e.type;if(f=="svg"?a="http://www.w3.org/2000/svg":f=="math"?a="http://www.w3.org/1998/Math/MathML":a||(a="http://www.w3.org/1999/xhtml"),r!=null){for(l=0;l<r.length;l++)if((p=r[l])&&"setAttribute"in p==!!f&&(f?p.localName==f:p.nodeType==3)){t=p,r[l]=null;break}}if(t==null){if(f==null)return document.createTextNode(y);t=document.createElementNS(a,f,y.is&&y),d&&(g.__m&&g.__m(e,r),d=!1),r=null}if(f==null)x===y||d&&t.data==y||(t.data=y);else{if(r=f=="textarea"&&y.defaultValue!=null?null:r&&G.call(t.childNodes),!d&&r!=null)for(x={},l=0;l<t.attributes.length;l++)x[(p=t.attributes[l]).name]=p.value;for(l in x)p=x[l],l=="dangerouslySetInnerHTML"?h=p:l=="children"||l in y||l=="value"&&"defaultValue"in y||l=="checked"&&"defaultChecked"in y||F(t,l,null,p,a);for(l in y)p=y[l],l=="children"?i=p:l=="dangerouslySetInnerHTML"?u=p:l=="value"?b=p:l=="checked"?k=p:d&&typeof p!="function"||x[l]===p||F(t,l,p,x[l],a);if(u)d||h&&(u.__html==h.__html||u.__html==t.innerHTML)||(t.innerHTML=u.__html),e.__k=[];else if(h&&(t.innerHTML=""),we(e.type=="template"?t.content:t,K(i)?i:[i],e,n,o,f=="foreignObject"?"http://www.w3.org/1999/xhtml":a,r,s,r?r[0]:n.__k&&O(n,0),d,_),r!=null)for(l=r.length;l--;)oe(r[l]);d&&f!="textarea"||(l="value",f=="progress"&&b==null?t.removeAttribute("value"):b!=null&&(b!==t[l]||f=="progress"&&!b||f=="option"&&b!=x[l])&&F(t,l,b,x[l],a),l="checked",k!=null&&k!=t[l]&&F(t,l,k,x[l],a))}return t}function ie(t,e,n){try{if(typeof t=="function"){var o=typeof t.__u=="function";o&&t.__u(),o&&e==null||(t.__u=t(e))}else t.current=e}catch(a){g.__e(a,n)}}function Pe(t,e,n){var o,a;if(g.unmount&&g.unmount(t),(o=t.ref)&&(o.current&&o.current!=t.__e||ie(o,null,e)),(o=t.__c)!=null){if(o.componentWillUnmount)try{o.componentWillUnmount()}catch(r){g.__e(r,e)}o.base=o.__P=o.__n=null}if(o=t.__k)for(a=0;a<o.length;a++)o[a]&&Pe(o[a],e,n||typeof t.type!="function");n||oe(t.__e),t.__c=t.__=t.__e=void 0}function pt(t,e,n){return this.constructor(t,n)}function Ae(t,e,n){var o,a,r,s;e==document&&(e=document.documentElement),g.__&&g.__(t,e),a=(o=typeof n=="function")?null:n&&n.__k||e.__k,r=[],s=[],ae(e,t=(!o&&n||e).__k=dt(A,null,[t]),a||z,z,e.namespaceURI,!o&&n?[n]:a?null:e.firstChild?G.call(e.childNodes):null,r,!o&&n?n:a?a.__e:e.firstChild,o,s),Ce(r,t,s),t.props.children=null}function De(t){function e(n){var o,a;return this.getChildContext||(o=new Set,(a={})[e.__c]=this,this.getChildContext=function(){return a},this.componentWillUnmount=function(){o=null},this.shouldComponentUpdate=function(r){this.props.value!=r.value&&o.forEach(function(s){s.__e=!0,ne(s)})},this.sub=function(r){o.add(r);var s=r.componentWillUnmount;r.componentWillUnmount=function(){o&&o.delete(r),s&&s.call(r)}}),n.children}return e.__c="__cC"+xe++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,o){return n.children(o)}).contextType=e,e}G=q.slice,g={__e:function(t,e,n,o){for(var a,r,s;e=e.__;)if((a=e.__c)&&!a.__)try{if((r=a.constructor)&&r.getDerivedStateFromError!=null&&(a.setState(r.getDerivedStateFromError(t)),s=a.__d),a.componentDidCatch!=null&&(a.componentDidCatch(t,o||{}),s=a.__d),s)return a.__E=a}catch(d){t=d}throw t}},me=0,it=function(t){return t!=null&&t.constructor===void 0},j.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=P({},this.state),typeof t=="function"&&(t=t(P({},n),this.props)),t&&P(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),ne(this))},j.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),ne(this))},j.prototype.render=A,D=[],be=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,ve=function(t,e){return t.__v.__b-e.__v.__b},V.__r=0,Z=Math.random().toString(8),U="__d"+Z,L="__a"+Z,ye=/(PointerCapture)$|Capture$/i,re=0,ee=he(!1),te=he(!0),xe=0;var H,m,se,Ie,Y=0,Re=[],v=g,Oe=v.__b,Ne=v.__r,He=v.diffed,$e=v.__c,Be=v.unmount,Le=v.__;function Q(t,e){v.__h&&v.__h(m,t,Y||e),Y=0;var n=m.__H||(m.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function M(t){return Y=1,gt(ze,t)}function gt(t,e,n){var o=Q(H++,2);if(o.t=t,!o.__c&&(o.__=[n?n(e):ze(void 0,e),function(d){var _=o.__N?o.__N[0]:o.__[0],l=o.t(_,d);_!==l&&(o.__N=[l,o.__[1]],o.__c.setState({}))}],o.__c=m,!m.__f)){var a=function(d,_,l){if(!o.__c.__H)return!0;var u=!1,h=o.__c.props!==d;if(o.__c.__H.__.some(function(p){if(p.__N){u=!0;var b=p.__[0];p.__=p.__N,p.__N=void 0,b!==p.__[0]&&(h=!0)}}),r){var i=r.call(this,d,_,l);return u?i||h:i}return!u||h};m.__f=!0;var r=m.shouldComponentUpdate,s=m.componentWillUpdate;m.componentWillUpdate=function(d,_,l){if(this.__e){var u=r;r=void 0,a(d,_,l),r=u}s&&s.call(this,d,_,l)},m.shouldComponentUpdate=a}return o.__N||o.__}function Fe(t,e){var n=Q(H++,4);!v.__s&&je(n.__H,e)&&(n.__=t,n.u=e,m.__h.push(n))}function Ue(t){return Y=5,ft(function(){return{current:t}},[])}function ft(t,e){var n=Q(H++,7);return je(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function We(t){var e=m.context[t.__c],n=Q(H++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(m)),e.props.value):t.__}function ht(){for(var t;t=Re.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(J),e.__h.some(de),e.__h=[]}catch(n){e.__h=[],v.__e(n,t.__v)}}}v.__b=function(t){m=null,Oe&&Oe(t)},v.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),Le&&Le(t,e)},v.__r=function(t){Ne&&Ne(t),H=0;var e=(m=t.__c).__H;e&&(se===m?(e.__h=[],m.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(J),e.__h.some(de),e.__h=[],H=0)),se=m},v.diffed=function(t){He&&He(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(Re.push(e)!==1&&Ie===v.requestAnimationFrame||((Ie=v.requestAnimationFrame)||mt)(ht)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),se=m=null},v.__c=function(t,e){e.some(function(n){try{n.__h.some(J),n.__h=n.__h.filter(function(o){return!o.__||de(o)})}catch(o){e.some(function(a){a.__h&&(a.__h=[])}),e=[],v.__e(o,n.__v)}}),$e&&$e(t,e)},v.unmount=function(t){Be&&Be(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(o){try{J(o)}catch(a){e=a}}),n.__H=void 0,e&&v.__e(e,n.__v))};var Me=typeof requestAnimationFrame=="function";function mt(t){var e,n=function(){clearTimeout(o),Me&&cancelAnimationFrame(e),setTimeout(t)},o=setTimeout(n,35);Me&&(e=requestAnimationFrame(n))}function J(t){var e=m,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),m=e}function de(t){var e=m;t.__c=t.__(),m=e}function je(t,e){return!t||t.length!==e.length||e.some(function(n,o){return n!==t[o]})}function ze(t,e){return typeof e=="function"?e(t):e}var qe="(prefers-color-scheme: dark)";function bt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function vt(t){let e=t;for(;e;){let n=bt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(qe).matches?"dark":"light"}function Ve(t){let[e,n]=M("light");return Fe(()=>{let o=t.current;if(!o)return;let a=()=>n(vt(o.parentElement||o));a();let r=new MutationObserver(a),s={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};r.observe(document.documentElement,s),document.body&&r.observe(document.body,s);let d=window.matchMedia?window.matchMedia(qe):null;return d&&d.addEventListener("change",a),()=>{r.disconnect(),d&&d.removeEventListener("change",a)}},[t]),e}var S={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",badgeRadius:13,badgeRadiusActive:15,badgeOnEdgeMin:60},yt=`
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
`,Ge=`
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
`,xt=`
  .dg-diagram { ${yt} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${Ge} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${Ge} }

  /* frame */
  .dg-diagram {
    background: var(--dg-bg);
    border: 1px solid var(--dg-border);
    border-radius: 12px;
    padding: 12px;
    margin: 1.5em auto;
    max-width: 816px;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  }
  .dg-diagram svg { width: 100%; height: auto; display: block; }

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

  /* badges: the interactive hotspots */
  .dg-badge { cursor: pointer; }
  .dg-badge circle { fill: var(--dg-badge); stroke: var(--dg-badge-stroke); }
  .dg-badge-active circle { fill: var(--dg-accent); stroke: var(--dg-accent-stroke); }
  .dg-badge:focus { outline: none; }
  .dg-badge:focus circle { stroke: var(--dg-accent-stroke); stroke-width: 2.5; }
  .dg-badge-label { fill: var(--dg-badge-text); font-size: 11px; font-weight: 700; pointer-events: none; }
  .dg-badge-active .dg-badge-label { fill: var(--dg-on-accent); }

  /* detail panel under the drawing */
  .dg-diagram .dg-panel {
    margin-top: 10px;
    padding: 12px 14px;
    background: var(--dg-panel);
    border: 1px solid var(--dg-border);
    border-radius: 8px;
    min-height: 64px;
  }
  .dg-diagram .dg-panel-title { color: var(--dg-panel-title); font-weight: 700; font-size: 14px; margin-bottom: 4px; }
  .dg-diagram .dg-panel-desc { color: var(--dg-panel-text); font-size: 13.5px; line-height: 1.5; margin: 0 0 8px; }
  .dg-diagram .dg-hint { color: var(--dg-hint); font-size: 13px; font-style: italic; margin: 0; }
  .dg-diagram .dg-chips { display: flex; flex-wrap: wrap; gap: 8px; }
  .dg-diagram .dg-chip { display: flex; align-items: baseline; gap: 5px; font-size: 11px; padding: 3px 10px; border-radius: 999px; border: 1px solid; }
  .dg-chip-id { font-weight: 700; }
  .dg-chip-desc { font-weight: 500; opacity: 0.85; }
  .dg-chip-info { color: var(--dg-chip-info); border-color: var(--dg-chip-info-border); background: var(--dg-chip-info-bg); }
  .dg-chip-danger { color: var(--dg-chip-danger); border-color: var(--dg-chip-danger-border); background: var(--dg-chip-danger-bg); }
  .dg-chip-safe { color: var(--dg-chip-safe); border-color: var(--dg-chip-safe-border); background: var(--dg-chip-safe-bg); }
  .dg-chip-neutral { color: var(--dg-chip-neutral); border-color: var(--dg-chip-neutral-border); background: var(--dg-chip-neutral-bg); }
`,Ke="dg-diagram-styles";function Je(){if(typeof document=="undefined"||document.getElementById(Ke))return;let t=document.createElement("style");t.id=Ke,t.textContent=xt,document.head.appendChild(t)}var kt=0,Lt=Array.isArray;function c(t,e,n,o,a,r){e||(e={});var s,d,_=e;if("ref"in _)for(d in _={},e)d=="ref"?s=e[d]:_[d]=e[d];var l={type:t,props:_,key:n,ref:s,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--kt,__i:-1,__u:0,__source:a,__self:r};if(typeof t=="function"&&(s=t.defaultProps))for(d in s)_[d]===void 0&&(_[d]=s[d]);return g.vnode&&g.vnode(l),l}var Qe=De({markerPrefix:"dg-arrow"});function St(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function Et(t,e="Edge"){for(let n=1;n<t.length;n++){let[o,a]=t[n-1],[r,s]=t[n];if(o!==r&&a!==s)throw new Error(`${e}: diagonal segment (${o},${a})\u2192(${r},${s}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function le({x:t,y:e,w:n,h:o,label:a,variant:r="neutral",filled:s}){return c("g",{class:`dg-outline dg-outline-${r}${s?" dg-outline-filled":""}`,children:[c("rect",{x:t,y:e,width:n,height:o,rx:S.outlineRadius,"stroke-width":S.outlineStroke,"stroke-dasharray":S.outlineDash}),a?c("text",{x:t+14,y:e+22,class:"dg-label dg-label-outline",children:a}):null]})}function w({x:t,y:e,w:n,h:o,label:a,sub:r,variant:s="default",small:d}){let _=s.startsWith("solid-"),l=r?e+o/2-6:e+o/2+5;return c("g",{class:`dg-box dg-box-${s}${_?" dg-box-solid":""}`,children:[c("rect",{x:t,y:e,width:n,height:o,rx:S.boxRadius,"stroke-width":S.boxStroke}),c("text",{x:t+n/2,y:l,"text-anchor":"middle",class:d?"dg-label dg-label-sm":"dg-label dg-label-bold",children:a}),r?c("text",{x:t+n/2,y:l+(d?14:18),"text-anchor":"middle",class:"dg-label dg-label-sub",children:r}):null]})}function ce({points:t,dashed:e,active:n,dir:o,id:a}){Et(t,a?`Edge ${a}`:"Edge");let{markerPrefix:r}=We(Qe),s=`${r}-${n?"active":"default"}`,d=o&&o!=="none";return c("polyline",{class:`dg-edge${n?" dg-edge-active":""}`,points:St(t),fill:"none","stroke-width":n?S.edgeStrokeActive:S.edgeStroke,"stroke-dasharray":e?S.edgeDash:void 0,"marker-end":d?`url(#${s})`:void 0,"marker-start":o==="both"?`url(#${s})`:void 0})}function Xe({id:t,label:e,name:n,x:o,y:a,active:r,pressed:s,onSelect:d,onHover:_}){let l=(u,h)=>{u.pointerType==="mouse"&&_&&_(h)};return c("g",{class:`dg-badge${r?" dg-badge-active":""}`,role:"button",tabindex:"0","aria-pressed":s,"aria-label":n?`${t}: ${n}`:t,onClick:()=>d(t),onPointerEnter:u=>l(u,t),onPointerLeave:u=>l(u,null),onKeyDown:u=>{(u.key==="Enter"||u.key===" ")&&(u.preventDefault(),d(t))},children:[c("circle",{cx:o,cy:a,r:r?S.badgeRadiusActive:S.badgeRadius,"stroke-width":"1.5"}),c("text",{x:o,y:a+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function Ze(){let[t,e]=M(null),[n,o]=M(null);return{shownId:n||t,pinnedId:t,select:r=>e(s=>s===r?null:r),setHover:o}}function Ct({tone:t="neutral",id:e,text:n}){return c("span",{class:`dg-chip dg-chip-${t}`,children:[c("span",{class:"dg-chip-id",children:e}),n?c("span",{class:"dg-chip-desc",children:n}):null]})}function et({id:t,title:e,desc:n,chips:o,hint:a,children:r}){return c("div",{class:"dg-panel","aria-live":"polite",children:e?c(A,{children:[c("div",{class:"dg-panel-title",children:[t?c(A,{children:[t," \u2014 "]}):null,e]}),n?c("div",{class:"dg-panel-desc",children:n}):null,o&&o.length?c("div",{class:"dg-chips",children:o.map(s=>c(Ct,{...s},`${s.tone}-${s.id}`))}):null,r]}):c("div",{class:"dg-hint",children:a})})}function Ye({id:t,active:e}){return c("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:c("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}function tt({id:t,viewH:e,viewW:n=960,label:o,footer:a,children:r}){Je();let s=Ue(null),d=Ve(s),_=`dg-arrow-${t}`;return c("div",{class:"dg-diagram","data-dg-theme":d,ref:s,children:[c(Qe.Provider,{value:{markerPrefix:_},children:c("svg",{viewBox:`0 0 ${n} ${e}`,role:"group","aria-label":o,children:[c("defs",{children:[c(Ye,{id:`${_}-default`}),c(Ye,{id:`${_}-active`,active:!0})]}),r]})}),a]})}var nt={"main.py":"input() \u21C4 interrupt()","n06_exchange.py":"exchange()","gateway.py":"send_chat_turn()","render.py":"TurnRenderer","container.py":"ExecSession(tty=False)","config.py":"CHAT_CMD",claude:"stream-json \u21C4 stream-json",API:"Messages API",cache:"prompt cache",history:"conversation history",auto:"--permission-mode auto"},_e={"B-1":{name:"Operator input",desc:"Every question the harness asks is a LangGraph interrupt(); main.py answers it with a bare input() and resumes the graph with Command(resume=text). The text is the only thing that crosses here.",chips:["main.py","n06_exchange.py"]},"B-2":{name:"Rendered output",desc:"render.py owns a rich Console built with THEME. Spinner, live tail, markdown, tool panels and the footer are printed straight to the terminal from inside the node \u2014 before the graph has even returned from the turn.",chips:["render.py"]},"B-3":{name:"Events \u2192 renderer",desc:'send_chat_turn() parses each NDJSON line into a dict and calls TurnRenderer.feed(ev). The renderer dispatches on ev["type"]: stream_event, assistant, user, result; anything else (system, rate_limit_event) is ignored.',chips:["gateway.py","render.py"]},"B-4":{name:"Session read/write",desc:'One ExecSession per container, kept in a module-level dict across graph passes and interrupts. write() sends one JSON user message; readline() returns one stdout line, None on timeout, "" on EOF.',chips:["gateway.py","container.py"]},"B-5":{name:"Exec socket (framed)",desc:'exec_create(stdin=True, tty=False) + exec_start(socket=True). Without a TTY, Docker multiplexes stdout and stderr on one socket with 8-byte frame headers; _demux() unpacks them (">BxxxL") and keeps partial frames until the rest arrives.',chips:["container.py"]},"B-6":{name:"Process stdin/stdout",desc:"The exec instance is a long-lived `claude -p` process: stdin is NDJSON user messages (--input-format stream-json), stdout is NDJSON events (--output-format stream-json --include-partial-messages). It stays alive until the harness closes stdin.",chips:["config.py","claude"]},"B-7":{name:"CLI \u21C4 Anthropic API",desc:"The CLI, not the harness, holds the conversation. Each turn it sends the same system prompt, the same tool definitions and the whole history so far, with cache_control breakpoints \u2014 so all but a handful of input tokens are prompt-cache reads.",chips:["claude","API","cache","history"]}};var Tt=430,E={terminal:{x:20,y:20,w:460,h:56},api:{x:780,y:20,w:160,h:56},main:{x:40,y:140,w:110,h:56},render:{x:170,y:140,w:130,h:56},n06:{x:320,y:140,w:140,h:56},gateway:{x:170,y:240,w:290,h:56},session:{x:170,y:330,w:290,h:56},engine:{x:520,y:150,w:120,h:70},claude:{x:700,y:150,w:210,h:70},history:{x:700,y:250,w:130,h:50},tools:{x:700,y:330,w:210,h:50}},rt={host:{x:20,y:110,w:460,h:300},container:{x:670,y:110,w:270,h:300}},ot=[{id:"B-1",points:[[95,140],[95,76]],badge:[95,108],dir:"both"},{id:"B-2",points:[[235,140],[235,76]],badge:[235,108],dir:"end"},{id:"B-3",points:[[235,240],[235,196]],badge:[257,218],dir:"end"},{id:"B-4",points:[[315,296],[315,330]],badge:[337,313],dir:"both"},{id:"B-5",points:[[460,358],[580,358],[580,220]],badge:[520,358],dir:"both"},{id:"B-6",points:[[640,185],[700,185]],badge:[670,185],dir:"both"},{id:"B-7",points:[[895,150],[895,76]],badge:[895,113],dir:"both"}],Pt=[{points:[[440,196],[440,240]],dir:"end"},{points:[[765,220],[765,250]],dashed:!0},{points:[[880,220],[880,330]],dir:"end",dashed:!0}];function ue({id:t="blocks"}){let{shownId:e,pinnedId:n,select:o,setHover:a}=Ze(),r=e?_e[e]:null,s=r?r.chips.map(d=>({tone:"info",id:d,text:nt[d]})):[];return c(tt,{id:t,viewH:Tt,label:"Block diagram: operator terminal, host process, Docker engine, persistent claude process in the sandbox, Anthropic API",footer:c(et,{id:e,title:r==null?void 0:r.name,desc:r==null?void 0:r.desc,chips:s,hint:"Hover or click a numbered badge to see what crosses that link and which file implements it."}),children:[c(le,{...rt.host,label:"HOST PROCESS \xB7 agent-sandbox",variant:"info"}),c(le,{...rt.container,label:"AGENT CONTAINER \xB7 runsc",variant:"safe",filled:!0}),c(w,{...E.terminal,label:"Operator terminal",sub:"stdin \u2190 input()   \xB7   stdout \u2190 rich Console"}),c(w,{...E.api,label:"Anthropic API",sub:"Messages \xB7 prompt cache"}),c(w,{...E.main,label:"main.py",sub:"graph.invoke()"}),c(w,{...E.render,label:"render.py",sub:"TurnRenderer \xB7 THEME"}),c(w,{...E.n06,label:"n06_exchange",sub:"graph node (loop)"}),c(w,{...E.gateway,label:"gateway.py",sub:"start_chat() \xB7 send_chat_turn() \xB7 close_chat()"}),c(w,{...E.session,label:"container.py \xB7 ExecSession",sub:"tty=False \xB7 readline() \xB7 _demux()"}),c(w,{...E.engine,label:"Docker Engine",sub:"exec API"}),c(w,{...E.claude,label:"claude -p",sub:"stream-json in \xB7 stream-json out",variant:"solid-safe"}),c(w,{...E.history,label:"conversation",sub:"in-process",small:!0}),c(w,{...E.tools,label:"tools \xB7 Bash, Read, Edit\u2026",sub:"--permission-mode auto",small:!0}),Pt.map((d,_)=>c(ce,{...d},_)),ot.map(d=>c(ce,{id:d.id,points:d.points,dir:d.dir,active:e===d.id},d.id)),ot.map(d=>c(Xe,{id:d.id,label:d.id.replace("B-",""),name:_e[d.id].name,x:d.badge[0],y:d.badge[1],active:e===d.id,pressed:n===d.id,onSelect:o,onHover:a},d.id))]})}var At={"island-blocks":{id:"blocks"}};function at(){for(let[t,e]of Object.entries(At)){let n=document.getElementById(t);n&&Ae(c(ue,{...e}),n)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",at):at();})();
