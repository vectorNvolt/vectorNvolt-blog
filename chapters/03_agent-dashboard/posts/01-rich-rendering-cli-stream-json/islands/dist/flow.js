(()=>{var V,g,he,st,C,pe,me,be,ee,B,L,ve,oe,te,ne,ye,j={},z=[],it=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,Y=Array.isArray;function E(t,e){for(var n in e)t[n]=e[n];return t}function ae(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function dt(t,e,n){var r,s,o,i={};for(o in e)o=="key"?r=e[o]:o=="ref"?s=e[o]:i[o]=e[o];if(arguments.length>2&&(i.children=arguments.length>3?V.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(o in t.defaultProps)i[o]===void 0&&(i[o]=t.defaultProps[o]);return U(t,i,r,s,null)}function U(t,e,n,r,s){var o={type:t,props:e,key:n,ref:r,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:s==null?++he:s,__i:-1,__u:0};return s==null&&g.vnode!=null&&g.vnode(o),o}function T(t){return t.children}function W(t,e){this.props=t,this.context=e}function A(t,e){if(e==null)return t.__?A(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?A(t):null}function lt(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,r=[],s=[],o=E({},e);o.__v=e.__v+1,g.vnode&&g.vnode(o),se(t.__P,o,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,r,n==null?A(e):n,!!(32&e.__u),s),o.__v=e.__v,o.__.__k[o.__i]=o,Ee(r,o,s),e.__e=e.__=null,o.__e!=n&&xe(o)}}function xe(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),xe(t)}function re(t){(!t.__d&&(t.__d=!0)&&C.push(t)&&!q.__r++||pe!=g.debounceRendering)&&((pe=g.debounceRendering)||me)(q)}function q(){try{for(var t,e=1;C.length;)C.length>e&&C.sort(be),t=C.shift(),e=C.length,lt(t)}finally{C.length=q.__r=0}}function ke(t,e,n,r,s,o,i,c,l,d,_){var h,a,u,m,k,x,y=r&&r.__k||z,f=e.length;for(l=ct(n,e,y,l,f),h=0;h<f;h++)(u=n.__k[h])!=null&&(a=u.__i!=-1&&y[u.__i]||j,u.__i=h,x=se(t,u,a,s,o,i,c,l,d,_),m=u.__e,u.ref&&a.ref!=u.ref&&(a.ref&&ie(a.ref,null,u),_.push(u.ref,u.__c||m,u)),k==null&&m!=null&&(k=m),4&u.__u?(l=we(u,l,t),a.__e&&(a.__e=null)):typeof u.type=="function"&&x!==void 0?l=x:m&&(l=m.nextSibling),u.__u&=-7);return n.__e=k,l}function ct(t,e,n,r,s){var o,i,c,l,d,_=n.length,h=_,a=0;for(t.__k=new Array(s),o=0;o<s;o++)(i=e[o])!=null&&typeof i!="boolean"&&typeof i!="function"?(typeof i=="string"||typeof i=="number"||typeof i=="bigint"||i.constructor==String?i=t.__k[o]=U(null,i,null,null,null):Y(i)?i=t.__k[o]=U(T,{children:i},null,null,null):i.constructor===void 0&&i.__b>0?i=t.__k[o]=U(i.type,i.props,i.key,i.ref?i.ref:null,i.__v):t.__k[o]=i,l=o+a,i.__=t,i.__b=t.__b+1,c=null,(d=i.__i=_t(i,n,l,h))!=-1&&(h--,(c=n[d])&&(c.__u|=2)),c==null||c.__v==null?(d==-1&&(s>_?a--:s<_&&a++),typeof i.type!="function"&&(i.__u|=4)):d!=l&&(d==l-1?a--:d==l+1?a++:(d>l?a--:a++,i.__u|=4))):t.__k[o]=null;if(h)for(o=0;o<_;o++)(c=n[o])!=null&&!(2&c.__u)&&(c.__e==r&&(r=A(c)),Ce(c,c));return r}function we(t,e,n){var r,s;if(typeof t.type=="function"){for(r=t.__k,s=0;r&&s<r.length;s++)r[s]&&(r[s].__=t,e=we(r[s],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=A(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function _t(t,e,n,r){var s,o,i,c=t.key,l=t.type,d=e[n],_=d!=null&&(2&d.__u)==0;if(d===null&&c==null||_&&c==d.key&&l==d.type)return n;if(r>(_?1:0)){for(s=n-1,o=n+1;s>=0||o<e.length;)if((d=e[i=s>=0?s--:o++])!=null&&!(2&d.__u)&&c==d.key&&l==d.type)return i}return-1}function ge(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||it.test(e)?n:n+"px"}function R(t,e,n,r,s){var o,i;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof r=="string"&&(t.style.cssText=r=""),r)for(e in r)n&&e in n||ge(t.style,e,"");if(n)for(e in n)r&&n[e]==r[e]||ge(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")o=e!=(e=e.replace(ve,"$1")),i=e.toLowerCase(),e=i in t||e=="onFocusOut"||e=="onFocusIn"?i.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+o]=n,n?r?n[L]=r[L]:(n[L]=oe,t.addEventListener(e,o?ne:te,o)):t.removeEventListener(e,o?ne:te,o);else{if(s=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function fe(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[B]==null)e[B]=oe++;else if(e[B]<n[L])return;return n(g.event?g.event(e):e)}}}function se(t,e,n,r,s,o,i,c,l,d){var _,h,a,u,m,k,x,y,f,w,H,I,$,ue,F,Z,S=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(l=!!(32&n.__u),o=[c=e.__e=n.__e]),(_=g.__b)&&_(e);e:if(typeof S=="function"){h=i.length;try{if(f=e.props,w=S.prototype&&S.prototype.render,H=(_=S.contextType)&&r[_.__c],I=_?H?H.props.value:_.__:r,n.__c?y=(a=e.__c=n.__c).__=a.__E:(w?e.__c=a=new S(f,I):(e.__c=a=new W(f,I),a.constructor=S,a.render=pt),H&&H.sub(a),a.state||(a.state={}),a.__n=r,u=a.__d=!0,a.__h=[],a._sb=[]),w&&a.__s==null&&(a.__s=a.state),w&&S.getDerivedStateFromProps!=null&&(a.__s==a.state&&(a.__s=E({},a.__s)),E(a.__s,S.getDerivedStateFromProps(f,a.__s))),m=a.props,k=a.state,a.__v=e,u)w&&S.getDerivedStateFromProps==null&&a.componentWillMount!=null&&a.componentWillMount(),w&&a.componentDidMount!=null&&a.__h.push(a.componentDidMount);else{if(w&&S.getDerivedStateFromProps==null&&f!==m&&a.componentWillReceiveProps!=null&&a.componentWillReceiveProps(f,I),e.__v==n.__v||!a.__e&&a.shouldComponentUpdate!=null&&a.shouldComponentUpdate(f,a.__s,I)===!1){e.__v!=n.__v&&(a.props=f,a.state=a.__s,a.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(D){D&&(D.__=e)}),z.push.apply(a.__h,a._sb),a._sb=[],a.__h.length&&i.push(a),c=A(n);break e}a.componentWillUpdate!=null&&a.componentWillUpdate(f,a.__s,I),w&&a.componentDidUpdate!=null&&a.__h.push(function(){a.componentDidUpdate(m,k,x)})}if(a.context=I,a.props=f,a.__P=t,a.__e=!1,$=g.__r,ue=0,w)a.state=a.__s,a.__d=!1,$&&$(e),_=a.render(a.props,a.state,a.context),z.push.apply(a.__h,a._sb),a._sb=[];else do a.__d=!1,$&&$(e),_=a.render(a.props,a.state,a.context),a.state=a.__s;while(a.__d&&++ue<25);a.state=a.__s,a.getChildContext!=null&&(r=E(E({},r),a.getChildContext())),w&&!u&&a.getSnapshotBeforeUpdate!=null&&(x=a.getSnapshotBeforeUpdate(m,k)),F=_!=null&&_.type===T&&_.key==null?Te(_.props.children):_,c=ke(t,Y(F)?F:[F],e,n,r,s,o,i,c,l,d),a.base=e.__e,e.__u&=-161,a.__h.length&&i.push(a),y&&(a.__E=a.__=null)}catch(D){if(i.length=h,e.__v=null,l||o!=null){if(D.then){for(e.__u|=l?160:128;c&&c.nodeType==8&&c.nextSibling;)c=c.nextSibling;o!=null&&(o[o.indexOf(c)]=null),e.__e=c}else if(o!=null)for(Z=o.length;Z--;)ae(o[Z])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),D.then||Se(e),g.__e(D,e,n)}}else o==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):c=e.__e=ut(n.__e,e,n,r,s,o,i,l,d);return(_=g.diffed)&&_(e),128&e.__u?void 0:c}function Se(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Se))}function Ee(t,e,n){for(var r=0;r<n.length;r++)ie(n[r],n[++r],n[++r]);g.__c&&g.__c(e,t),t.some(function(s){try{t=s.__h,s.__h=[],t.some(function(o){o.call(s)})}catch(o){g.__e(o,s.__v)}})}function Te(t){return typeof t!="object"||t==null||t.__b>0?t:Y(t)?t.map(Te):t.constructor!==void 0?null:E({},t)}function ut(t,e,n,r,s,o,i,c,l){var d,_,h,a,u,m,k,x=n.props||j,y=e.props,f=e.type;if(f=="svg"?s="http://www.w3.org/2000/svg":f=="math"?s="http://www.w3.org/1998/Math/MathML":s||(s="http://www.w3.org/1999/xhtml"),o!=null){for(d=0;d<o.length;d++)if((u=o[d])&&"setAttribute"in u==!!f&&(f?u.localName==f:u.nodeType==3)){t=u,o[d]=null;break}}if(t==null){if(f==null)return document.createTextNode(y);t=document.createElementNS(s,f,y.is&&y),c&&(g.__m&&g.__m(e,o),c=!1),o=null}if(f==null)x===y||c&&t.data==y||(t.data=y);else{if(o=f=="textarea"&&y.defaultValue!=null?null:o&&V.call(t.childNodes),!c&&o!=null)for(x={},d=0;d<t.attributes.length;d++)x[(u=t.attributes[d]).name]=u.value;for(d in x)u=x[d],d=="dangerouslySetInnerHTML"?h=u:d=="children"||d in y||d=="value"&&"defaultValue"in y||d=="checked"&&"defaultChecked"in y||R(t,d,null,u,s);for(d in y)u=y[d],d=="children"?a=u:d=="dangerouslySetInnerHTML"?_=u:d=="value"?m=u:d=="checked"?k=u:c&&typeof u!="function"||x[d]===u||R(t,d,u,x[d],s);if(_)c||h&&(_.__html==h.__html||_.__html==t.innerHTML)||(t.innerHTML=_.__html),e.__k=[];else if(h&&(t.innerHTML=""),ke(e.type=="template"?t.content:t,Y(a)?a:[a],e,n,r,f=="foreignObject"?"http://www.w3.org/1999/xhtml":s,o,i,o?o[0]:n.__k&&A(n,0),c,l),o!=null)for(d=o.length;d--;)ae(o[d]);c&&f!="textarea"||(d="value",f=="progress"&&m==null?t.removeAttribute("value"):m!=null&&(m!==t[d]||f=="progress"&&!m||f=="option"&&m!=x[d])&&R(t,d,m,x[d],s),d="checked",k!=null&&k!=t[d]&&R(t,d,k,x[d],s))}return t}function ie(t,e,n){try{if(typeof t=="function"){var r=typeof t.__u=="function";r&&t.__u(),r&&e==null||(t.__u=t(e))}else t.current=e}catch(s){g.__e(s,n)}}function Ce(t,e,n){var r,s;if(g.unmount&&g.unmount(t),(r=t.ref)&&(r.current&&r.current!=t.__e||ie(r,null,e)),(r=t.__c)!=null){if(r.componentWillUnmount)try{r.componentWillUnmount()}catch(o){g.__e(o,e)}r.base=r.__P=r.__n=null}if(r=t.__k)for(s=0;s<r.length;s++)r[s]&&Ce(r[s],e,n||typeof t.type!="function");n||ae(t.__e),t.__c=t.__=t.__e=void 0}function pt(t,e,n){return this.constructor(t,n)}function Pe(t,e,n){var r,s,o,i;e==document&&(e=document.documentElement),g.__&&g.__(t,e),s=(r=typeof n=="function")?null:n&&n.__k||e.__k,o=[],i=[],se(e,t=(!r&&n||e).__k=dt(T,null,[t]),s||j,j,e.namespaceURI,!r&&n?[n]:s?null:e.firstChild?V.call(e.childNodes):null,o,!r&&n?n:s?s.__e:e.firstChild,r,i),Ee(o,t,i),t.props.children=null}function Ie(t){function e(n){var r,s;return this.getChildContext||(r=new Set,(s={})[e.__c]=this,this.getChildContext=function(){return s},this.componentWillUnmount=function(){r=null},this.shouldComponentUpdate=function(o){this.props.value!=o.value&&r.forEach(function(i){i.__e=!0,re(i)})},this.sub=function(o){r.add(o);var i=o.componentWillUnmount;o.componentWillUnmount=function(){r&&r.delete(o),i&&i.call(o)}}),n.children}return e.__c="__cC"+ye++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,r){return n.children(r)}).contextType=e,e}V=z.slice,g={__e:function(t,e,n,r){for(var s,o,i;e=e.__;)if((s=e.__c)&&!s.__)try{if((o=s.constructor)&&o.getDerivedStateFromError!=null&&(s.setState(o.getDerivedStateFromError(t)),i=s.__d),s.componentDidCatch!=null&&(s.componentDidCatch(t,r||{}),i=s.__d),i)return s.__E=s}catch(c){t=c}throw t}},he=0,st=function(t){return t!=null&&t.constructor===void 0},W.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=E({},this.state),typeof t=="function"&&(t=t(E({},n),this.props)),t&&E(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),re(this))},W.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),re(this))},W.prototype.render=T,C=[],me=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,be=function(t,e){return t.__v.__b-e.__v.__b},q.__r=0,ee=Math.random().toString(8),B="__d"+ee,L="__a"+ee,ve=/(PointerCapture)$|Capture$/i,oe=0,te=fe(!1),ne=fe(!0),ye=0;var O,b,de,Ae,K=0,Fe=[],v=g,De=v.__b,Oe=v.__r,He=v.diffed,$e=v.__c,Le=v.unmount,Ne=v.__;function G(t,e){v.__h&&v.__h(b,t,K||e),K=0;var n=b.__H||(b.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function N(t){return K=1,gt(je,t)}function gt(t,e,n){var r=G(O++,2);if(r.t=t,!r.__c&&(r.__=[n?n(e):je(void 0,e),function(c){var l=r.__N?r.__N[0]:r.__[0],d=r.t(l,c);l!==d&&(r.__N=[d,r.__[1]],r.__c.setState({}))}],r.__c=b,!b.__f)){var s=function(c,l,d){if(!r.__c.__H)return!0;var _=!1,h=r.__c.props!==c;if(r.__c.__H.__.some(function(u){if(u.__N){_=!0;var m=u.__[0];u.__=u.__N,u.__N=void 0,m!==u.__[0]&&(h=!0)}}),o){var a=o.call(this,c,l,d);return _?a||h:a}return!_||h};b.__f=!0;var o=b.shouldComponentUpdate,i=b.componentWillUpdate;b.componentWillUpdate=function(c,l,d){if(this.__e){var _=o;o=void 0,s(c,l,d),o=_}i&&i.call(this,c,l,d)},b.shouldComponentUpdate=s}return r.__N||r.__}function Re(t,e){var n=G(O++,4);!v.__s&&We(n.__H,e)&&(n.__=t,n.u=e,b.__h.push(n))}function Be(t){return K=5,ft(function(){return{current:t}},[])}function ft(t,e){var n=G(O++,7);return We(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function Ue(t){var e=b.context[t.__c],n=G(O++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(b)),e.props.value):t.__}function ht(){for(var t;t=Fe.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(J),e.__h.some(le),e.__h=[]}catch(n){e.__h=[],v.__e(n,t.__v)}}}v.__b=function(t){b=null,De&&De(t)},v.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),Ne&&Ne(t,e)},v.__r=function(t){Oe&&Oe(t),O=0;var e=(b=t.__c).__H;e&&(de===b?(e.__h=[],b.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(J),e.__h.some(le),e.__h=[],O=0)),de=b},v.diffed=function(t){He&&He(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(Fe.push(e)!==1&&Ae===v.requestAnimationFrame||((Ae=v.requestAnimationFrame)||mt)(ht)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),de=b=null},v.__c=function(t,e){e.some(function(n){try{n.__h.some(J),n.__h=n.__h.filter(function(r){return!r.__||le(r)})}catch(r){e.some(function(s){s.__h&&(s.__h=[])}),e=[],v.__e(r,n.__v)}}),$e&&$e(t,e)},v.unmount=function(t){Le&&Le(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(r){try{J(r)}catch(s){e=s}}),n.__H=void 0,e&&v.__e(e,n.__v))};var Me=typeof requestAnimationFrame=="function";function mt(t){var e,n=function(){clearTimeout(r),Me&&cancelAnimationFrame(e),setTimeout(t)},r=setTimeout(n,35);Me&&(e=requestAnimationFrame(n))}function J(t){var e=b,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),b=e}function le(t){var e=b;t.__c=t.__(),b=e}function We(t,e){return!t||t.length!==e.length||e.some(function(n,r){return n!==t[r]})}function je(t,e){return typeof e=="function"?e(t):e}var ze="(prefers-color-scheme: dark)";function bt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function vt(t){let e=t;for(;e;){let n=bt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(ze).matches?"dark":"light"}function qe(t){let[e,n]=N("light");return Re(()=>{let r=t.current;if(!r)return;let s=()=>n(vt(r.parentElement||r));s();let o=new MutationObserver(s),i={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};o.observe(document.documentElement,i),document.body&&o.observe(document.body,i);let c=window.matchMedia?window.matchMedia(ze):null;return c&&c.addEventListener("change",s),()=>{o.disconnect(),c&&c.removeEventListener("change",s)}},[t]),e}var P={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",badgeRadius:13,badgeRadiusActive:15,badgeOnEdgeMin:60},yt=`
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
`,Ve=`
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
    .dg-diagram:not([data-dg-theme="light"]) { ${Ve} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${Ve} }

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
`,Ye="dg-diagram-styles";function Je(){if(typeof document=="undefined"||document.getElementById(Ye))return;let t=document.createElement("style");t.id=Ye,t.textContent=xt,document.head.appendChild(t)}var kt=0,Bt=Array.isArray;function p(t,e,n,r,s,o){e||(e={});var i,c,l=e;if("ref"in l)for(c in l={},e)c=="ref"?i=e[c]:l[c]=e[c];var d={type:t,props:l,key:n,ref:i,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--kt,__i:-1,__u:0,__source:s,__self:o};if(typeof t=="function"&&(i=t.defaultProps))for(c in i)l[c]===void 0&&(l[c]=i[c]);return g.vnode&&g.vnode(d),d}var Ge=Ie({markerPrefix:"dg-arrow"});function St(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function Et(t,e="Edge"){for(let n=1;n<t.length;n++){let[r,s]=t[n-1],[o,i]=t[n];if(r!==o&&s!==i)throw new Error(`${e}: diagonal segment (${r},${s})\u2192(${o},${i}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function Qe({x:t,y:e,w:n,h:r,label:s,sub:o,variant:i="default",small:c}){let l=i.startsWith("solid-"),d=o?e+r/2-6:e+r/2+5;return p("g",{class:`dg-box dg-box-${i}${l?" dg-box-solid":""}`,children:[p("rect",{x:t,y:e,width:n,height:r,rx:P.boxRadius,"stroke-width":P.boxStroke}),p("text",{x:t+n/2,y:d,"text-anchor":"middle",class:c?"dg-label dg-label-sm":"dg-label dg-label-bold",children:s}),o?p("text",{x:t+n/2,y:d+(c?14:18),"text-anchor":"middle",class:"dg-label dg-label-sub",children:o}):null]})}function Q({points:t,dashed:e,active:n,dir:r,id:s}){Et(t,s?`Edge ${s}`:"Edge");let{markerPrefix:o}=Ue(Ge),i=`${o}-${n?"active":"default"}`,c=r&&r!=="none";return p("polyline",{class:`dg-edge${n?" dg-edge-active":""}`,points:St(t),fill:"none","stroke-width":n?P.edgeStrokeActive:P.edgeStroke,"stroke-dasharray":e?P.edgeDash:void 0,"marker-end":c?`url(#${i})`:void 0,"marker-start":r==="both"?`url(#${i})`:void 0})}function Xe({id:t,label:e,name:n,x:r,y:s,active:o,pressed:i,onSelect:c,onHover:l}){let d=(_,h)=>{_.pointerType==="mouse"&&l&&l(h)};return p("g",{class:`dg-badge${o?" dg-badge-active":""}`,role:"button",tabindex:"0","aria-pressed":i,"aria-label":n?`${t}: ${n}`:t,onClick:()=>c(t),onPointerEnter:_=>d(_,t),onPointerLeave:_=>d(_,null),onKeyDown:_=>{(_.key==="Enter"||_.key===" ")&&(_.preventDefault(),c(t))},children:[p("circle",{cx:r,cy:s,r:o?P.badgeRadiusActive:P.badgeRadius,"stroke-width":"1.5"}),p("text",{x:r,y:s+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function Ze(){let[t,e]=N(null),[n,r]=N(null);return{shownId:n||t,pinnedId:t,select:o=>e(i=>i===o?null:o),setHover:r}}function Tt({tone:t="neutral",id:e,text:n}){return p("span",{class:`dg-chip dg-chip-${t}`,children:[p("span",{class:"dg-chip-id",children:e}),n?p("span",{class:"dg-chip-desc",children:n}):null]})}function et({id:t,title:e,desc:n,chips:r,hint:s,children:o}){return p("div",{class:"dg-panel","aria-live":"polite",children:e?p(T,{children:[p("div",{class:"dg-panel-title",children:[t?p(T,{children:[t," \u2014 "]}):null,e]}),n?p("div",{class:"dg-panel-desc",children:n}):null,r&&r.length?p("div",{class:"dg-chips",children:r.map(i=>p(Tt,{...i},`${i.tone}-${i.id}`))}):null,o]}):p("div",{class:"dg-hint",children:s})})}function Ke({id:t,active:e}){return p("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:p("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}function tt({id:t,viewH:e,viewW:n=960,label:r,footer:s,children:o}){Je();let i=Be(null),c=qe(i),l=`dg-arrow-${t}`;return p("div",{class:"dg-diagram","data-dg-theme":c,ref:i,children:[p(Ge.Provider,{value:{markerPrefix:l},children:p("svg",{viewBox:`0 0 ${n} ${e}`,role:"group","aria-label":r,children:[p("defs",{children:[p(Ke,{id:`${l}-default`}),p(Ke,{id:`${l}-active`,active:!0})]}),o]})}),s]})}var nt={"main.py":"input() \u21C4 interrupt()","n06_exchange.py":"exchange()","gateway.py":"send_chat_turn()","render.py":"TurnRenderer","container.py":"ExecSession(tty=False)","config.py":"CHAT_CMD",claude:"stream-json \u21C4 stream-json",API:"Messages API",cache:"prompt cache",history:"conversation history",auto:"--permission-mode auto"};var ce={"S-1":{name:"Operator reply resumes the graph",desc:'The previous pass ended on interrupt("Your reply (or /quit):"). main.py collects the answer with input() and invokes the graph with Command(resume=text); exchange() runs again with turn == "agent" and picks the last user message out of state["transcript"].',chips:["main.py","n06_exchange.py"]},"S-2":{name:"One JSON line in",desc:'send_chat_turn() writes {"type":"user","message":{"role":"user","content":text},...} plus "\\n" to the session socket. On the first pass start_chat() opened the session first; afterwards the same process is reused.',chips:["gateway.py","container.py"]},"S-3":{name:"CLI calls the API with the full history",desc:"The CLI appends the message to its in-process conversation and sends system prompt + tools + every previous turn. Because that prefix is identical to the previous request plus an append, the API serves it from the prompt cache.",chips:["claude","API","cache"]},"S-4":{name:"stream_event deltas \u2192 live tail",desc:"content_block_start(text) stops the spinner and starts a transient rich Live; each content_block_delta appends to the tail (last N lines, plain text, blue). content_block_stop stops the Live and prints the whole block once as themed Markdown.",chips:["render.py"]},"S-5":{name:"assistant(tool_use) \u2192 cyan panel",desc:'The finished assistant message lists its content blocks. A tool_use block is printed as a Panel titled "tool \xB7 <name>" with the JSON input (clipped at 1500 chars), then the spinner says "running tool\u2026".',chips:["render.py"]},"S-6":{name:"Tool runs inside the container",desc:"With --permission-prompts none nobody can answer a prompt, so --permission-mode auto lets a classifier approve safe calls and deny the rest. Either way the tool executes inside the gVisor sandbox, never on the host.",chips:["auto","claude"]},"S-7":{name:"user(tool_result) \u2192 grey / red panel",desc:'Tool results arrive as a "user" event. Each tool_result block becomes a Panel (grey, or red with "\xB7 error" when is_error is set), clipped at 1200 chars; the spinner returns to "thinking\u2026" while the CLI calls the API again.',chips:["render.py"]},"S-8":{name:"result \u2192 footer, answer returned",desc:'The result event ends the turn: the renderer prints "<secs>s \xB7 <num_turns> turn(s)" and send_chat_turn() returns ev["result"]. exchange() appends it to state["transcript"] as an agent message and flips turn to "user".',chips:["render.py","gateway.py","n06_exchange.py"]},"S-9":{name:"interrupt() for the next reply",desc:"The next pass of exchange() runs ask_user() first \u2014 an interrupt() \u2014 so the graph pauses with the CLI process still alive and its history intact. /quit closes stdin, waits for EOF and drops the session.",chips:["n06_exchange.py","gateway.py"]}};var Ct=780,Pt=64,It=760,X={operator:{x:80,label:"Operator",sub:"terminal"},n06:{x:240,label:"n06_exchange",sub:"graph node"},gateway:{x:400,label:"gateway.py",sub:"+ TurnRenderer"},session:{x:560,label:"ExecSession",sub:"tty=False"},claude:{x:720,label:"claude -p",sub:"in the sandbox"},api:{x:880,label:"Anthropic API",sub:"prompt cache"}},rt=140,At=44,Dt=[{from:"operator",to:"n06",y:104,label:"reply text \u2192 Command(resume=\u2026)",badge:"S-1"},{from:"n06",to:"gateway",y:138,label:"send_chat_turn(c, last user msg)"},{from:"gateway",to:"claude",y:172,label:'write({"type":"user", \u2026} + "\\n")',badge:"S-2"},{from:"claude",to:"api",y:206,label:"POST: system + tools + full history",badge:"S-3"},{from:"api",to:"claude",y:240,label:"SSE stream"},{from:"claude",to:"gateway",y:274,label:"stream_event \xB7 content_block_delta \xD7N",badge:"S-4"},{from:"gateway",to:"operator",y:308,label:"Live tail \u2192 Markdown"},{from:"claude",to:"gateway",y:342,label:"assistant \xB7 tool_use",badge:"S-5"},{from:"gateway",to:"operator",y:376,label:"Panel  tool \xB7 Bash"},{from:"claude",to:"gateway",y:464,label:"user \xB7 tool_result",badge:"S-7"},{from:"gateway",to:"operator",y:498,label:"Panel  tool result"},{from:"claude",to:"api",y:532,label:"2nd call: history + tool_result \u2014 prefix cached",dir:"both"},{from:"claude",to:"gateway",y:566,label:"stream_event \xD7N \xB7 assistant \xB7 text"},{from:"gateway",to:"operator",y:600,label:"Markdown"},{from:"claude",to:"gateway",y:634,label:"result \xB7 usage \xB7 duration_ms",badge:"S-8"},{from:"gateway",to:"operator",y:668,label:"footer  7.1s \xB7 2 turn(s)"},{from:"gateway",to:"n06",y:702,label:"return answer \u2192 transcript"},{from:"n06",to:"operator",y:736,label:'interrupt("Your reply (or /quit):")',badge:"S-9"}],M={points:[[720,400],[790,400],[790,430],[720,430]],badge:[815,415],label:"runs the tool",at:[755,392]};function ot({text:t,at:e,anchor:n="middle"}){let[r,s]=e;return p("text",{x:r,y:s,"text-anchor":n,class:"dg-label dg-label-sub",children:t})}function _e({id:t="flow"}){let{shownId:e,pinnedId:n,select:r,setHover:s}=Ze(),o=e?ce[e]:null,i=o?o.chips.map(l=>({tone:"info",id:l,text:nt[l]})):[],c=[];return p(tt,{id:t,viewH:Ct,label:"Sequence diagram of one exchange turn with a tool call, from the operator's reply to the next interrupt",footer:p(et,{id:e,title:o==null?void 0:o.name,desc:o==null?void 0:o.desc,chips:i,hint:"Hover or click a numbered step to see what happens there and which file does it."}),children:[Object.entries(X).map(([l,d])=>p("g",{children:[p(Qe,{x:d.x-rt/2,y:20,w:rt,h:At,label:d.label,sub:d.sub,small:!0,variant:l==="claude"?"solid-safe":"default"}),p(Q,{id:`life-${l}`,points:[[d.x,Pt],[d.x,It]],dashed:!0})]},l)),Dt.map((l,d)=>{let _=X[l.from].x,h=X[l.to].x,a=(_+h)/2;Object.values(X).some(m=>m.x===a)&&(a-=80);let u=l.badge&&e===l.badge;return l.badge&&c.push({id:l.badge,x:a,y:l.y}),p("g",{children:[p(Q,{id:`msg-${d}`,points:[[_,l.y],[h,l.y]],dir:l.dir||"end",active:u}),p(ot,{text:l.label,at:[a,l.y-(l.badge?18:6)]})]},d)}),p(Q,{id:"self-loop",points:M.points,dir:"end",active:e==="S-6"}),p(ot,{text:M.label,at:M.at,anchor:"start"}),[...c,{id:"S-6",x:M.badge[0],y:M.badge[1]}].sort((l,d)=>l.id.localeCompare(d.id)).map(l=>p(Xe,{id:l.id,label:l.id.replace("S-",""),name:ce[l.id].name,x:l.x,y:l.y,active:e===l.id,pressed:n===l.id,onSelect:r,onHover:s},l.id))]})}var Ot={"island-flow":{id:"flow"}};function at(){for(let[t,e]of Object.entries(Ot)){let n=document.getElementById(t);n&&Pe(p(_e,{...e}),n)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",at):at();})();
