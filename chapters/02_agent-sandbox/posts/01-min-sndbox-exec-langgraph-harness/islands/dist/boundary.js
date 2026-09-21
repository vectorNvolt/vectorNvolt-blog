(()=>{var q,p,me,at,N,pe,be,ve,te,W,$,ye,ae,ne,re,xe,z={},V=[],it=/acit|ex(?:s|g|n|p|$)|rph|grid|ows|mnc|ntw|ine[ch]|zoo|^ord|itera/i,Y=Array.isArray;function S(t,e){for(var n in e)t[n]=e[n];return t}function ie(t){t&&t.parentNode&&t.parentNode.removeChild(t)}function st(t,e,n){var r,i,o,a={};for(o in e)o=="key"?r=e[o]:o=="ref"?i=e[o]:a[o]=e[o];if(arguments.length>2&&(a.children=arguments.length>3?q.call(arguments,2):n),typeof t=="function"&&t.defaultProps!=null)for(o in t.defaultProps)a[o]===void 0&&(a[o]=t.defaultProps[o]);return B(t,a,r,i,null)}function B(t,e,n,r,i){var o={type:t,props:e,key:n,ref:r,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:i==null?++me:i,__i:-1,__u:0};return i==null&&p.vnode!=null&&p.vnode(o),o}function E(t){return t.children}function j(t,e){this.props=t,this.context=e}function D(t,e){if(e==null)return t.__?D(t.__,t.__i+1):null;for(var n;e<t.__k.length;e++)if((n=t.__k[e])!=null&&n.__e!=null)return n.__e;return typeof t.type=="function"?D(t):null}function dt(t){if(t.__P&&t.__d){var e=t.__v,n=e.__e,r=[],i=[],o=S({},e);o.__v=e.__v+1,p.vnode&&p.vnode(o),se(t.__P,o,e,t.__n,t.__P.namespaceURI,32&e.__u?[n]:null,r,n==null?D(e):n,!!(32&e.__u),i),o.__v=e.__v,o.__.__k[o.__i]=o,Ie(r,o,i),e.__e=e.__=null,o.__e!=n&&ke(o)}}function ke(t){if((t=t.__)!=null&&t.__c!=null)return t.__e=t.__c.base=null,t.__k.some(function(e){if(e!=null&&e.__e!=null)return t.__e=t.__c.base=e.__e}),ke(t)}function oe(t){(!t.__d&&(t.__d=!0)&&N.push(t)&&!K.__r++||pe!=p.debounceRendering)&&((pe=p.debounceRendering)||be)(K)}function K(){try{for(var t,e=1;N.length;)N.length>e&&N.sort(ve),t=N.shift(),e=N.length,dt(t)}finally{N.length=K.__r=0}}function Te(t,e,n,r,i,o,a,l,_,d,c){var h,s,g,b,T,x,y=r&&r.__k||V,f=e.length;for(_=lt(n,e,y,_,f),h=0;h<f;h++)(g=n.__k[h])!=null&&(s=g.__i!=-1&&y[g.__i]||z,g.__i=h,x=se(t,g,s,i,o,a,l,_,d,c),b=g.__e,g.ref&&s.ref!=g.ref&&(s.ref&&de(s.ref,null,g),c.push(g.ref,g.__c||b,g)),T==null&&b!=null&&(T=b),4&g.__u?(_=we(g,_,t),s.__e&&(s.__e=null)):typeof g.type=="function"&&x!==void 0?_=x:b&&(_=b.nextSibling),g.__u&=-7);return n.__e=T,_}function lt(t,e,n,r,i){var o,a,l,_,d,c=n.length,h=c,s=0;for(t.__k=new Array(i),o=0;o<i;o++)(a=e[o])!=null&&typeof a!="boolean"&&typeof a!="function"?(typeof a=="string"||typeof a=="number"||typeof a=="bigint"||a.constructor==String?a=t.__k[o]=B(null,a,null,null,null):Y(a)?a=t.__k[o]=B(E,{children:a},null,null,null):a.constructor===void 0&&a.__b>0?a=t.__k[o]=B(a.type,a.props,a.key,a.ref?a.ref:null,a.__v):t.__k[o]=a,_=o+s,a.__=t,a.__b=t.__b+1,l=null,(d=a.__i=ct(a,n,_,h))!=-1&&(h--,(l=n[d])&&(l.__u|=2)),l==null||l.__v==null?(d==-1&&(i>c?s--:i<c&&s++),typeof a.type!="function"&&(a.__u|=4)):d!=_&&(d==_-1?s--:d==_+1?s++:(d>_?s--:s++,a.__u|=4))):t.__k[o]=null;if(h)for(o=0;o<c;o++)(l=n[o])!=null&&!(2&l.__u)&&(l.__e==r&&(r=D(l)),Se(l,l));return r}function we(t,e,n){var r,i;if(typeof t.type=="function"){for(r=t.__k,i=0;r&&i<r.length;i++)r[i]&&(r[i].__=t,e=we(r[i],e,n));return e}t.__e!=e&&(e&&t.type&&!e.parentNode&&(e=D(t)),e=n.insertBefore(t.__e,e||null));do e=e&&e.nextSibling;while(e!=null&&e.nodeType==8);return e}function ct(t,e,n,r){var i,o,a,l=t.key,_=t.type,d=e[n],c=d!=null&&(2&d.__u)==0;if(d===null&&l==null||c&&l==d.key&&_==d.type)return n;if(r>(c?1:0)){for(i=n-1,o=n+1;i>=0||o<e.length;)if((d=e[a=i>=0?i--:o++])!=null&&!(2&d.__u)&&l==d.key&&_==d.type)return a}return-1}function fe(t,e,n){e[0]=="-"?t.setProperty(e,n==null?"":n):t[e]=n==null?"":typeof n!="number"||it.test(e)?n:n+"px"}function F(t,e,n,r,i){var o,a;e:if(e=="style")if(typeof n=="string")t.style.cssText=n;else{if(typeof r=="string"&&(t.style.cssText=r=""),r)for(e in r)n&&e in n||fe(t.style,e,"");if(n)for(e in n)r&&n[e]==r[e]||fe(t.style,e,n[e])}else if(e[0]=="o"&&e[1]=="n")o=e!=(e=e.replace(ye,"$1")),a=e.toLowerCase(),e=a in t||e=="onFocusOut"||e=="onFocusIn"?a.slice(2):e.slice(2),t.l||(t.l={}),t.l[e+o]=n,n?r?n[$]=r[$]:(n[$]=ae,t.addEventListener(e,o?re:ne,o)):t.removeEventListener(e,o?re:ne,o);else{if(i=="http://www.w3.org/2000/svg")e=e.replace(/xlink(H|:h)/,"h").replace(/sName$/,"s");else if(e!="width"&&e!="height"&&e!="href"&&e!="list"&&e!="form"&&e!="tabIndex"&&e!="download"&&e!="rowSpan"&&e!="colSpan"&&e!="role"&&e!="popover"&&e in t)try{t[e]=n==null?"":n;break e}catch{}typeof n=="function"||(n==null||n===!1&&e[4]!="-"?t.removeAttribute(e):t.setAttribute(e,e=="popover"&&n==1?"":n))}}function he(t){return function(e){if(this.l){var n=this.l[e.type+t];if(e[W]==null)e[W]=ae++;else if(e[W]<n[$])return;return n(p.event?p.event(e):e)}}}function se(t,e,n,r,i,o,a,l,_,d){var c,h,s,g,b,T,x,y,f,I,R,A,H,ge,L,ee,C=e.type;if(e.constructor!==void 0)return null;128&n.__u&&(_=!!(32&n.__u),o=[l=e.__e=n.__e]),(c=p.__b)&&c(e);e:if(typeof C=="function"){h=a.length;try{if(f=e.props,I=C.prototype&&C.prototype.render,R=(c=C.contextType)&&r[c.__c],A=c?R?R.props.value:c.__:r,n.__c?y=(s=e.__c=n.__c).__=s.__E:(I?e.__c=s=new C(f,A):(e.__c=s=new j(f,A),s.constructor=C,s.render=ut),R&&R.sub(s),s.state||(s.state={}),s.__n=r,g=s.__d=!0,s.__h=[],s._sb=[]),I&&s.__s==null&&(s.__s=s.state),I&&C.getDerivedStateFromProps!=null&&(s.__s==s.state&&(s.__s=S({},s.__s)),S(s.__s,C.getDerivedStateFromProps(f,s.__s))),b=s.props,T=s.state,s.__v=e,g)I&&C.getDerivedStateFromProps==null&&s.componentWillMount!=null&&s.componentWillMount(),I&&s.componentDidMount!=null&&s.__h.push(s.componentDidMount);else{if(I&&C.getDerivedStateFromProps==null&&f!==b&&s.componentWillReceiveProps!=null&&s.componentWillReceiveProps(f,A),e.__v==n.__v||!s.__e&&s.shouldComponentUpdate!=null&&s.shouldComponentUpdate(f,s.__s,A)===!1){e.__v!=n.__v&&(s.props=f,s.state=s.__s,s.__d=!1),e.__e=n.__e,e.__k=n.__k,e.__k.some(function(U){U&&(U.__=e)}),V.push.apply(s.__h,s._sb),s._sb=[],s.__h.length&&a.push(s),l=D(n);break e}s.componentWillUpdate!=null&&s.componentWillUpdate(f,s.__s,A),I&&s.componentDidUpdate!=null&&s.__h.push(function(){s.componentDidUpdate(b,T,x)})}if(s.context=A,s.props=f,s.__P=t,s.__e=!1,H=p.__r,ge=0,I)s.state=s.__s,s.__d=!1,H&&H(e),c=s.render(s.props,s.state,s.context),V.push.apply(s.__h,s._sb),s._sb=[];else do s.__d=!1,H&&H(e),c=s.render(s.props,s.state,s.context),s.state=s.__s;while(s.__d&&++ge<25);s.state=s.__s,s.getChildContext!=null&&(r=S(S({},r),s.getChildContext())),I&&!g&&s.getSnapshotBeforeUpdate!=null&&(x=s.getSnapshotBeforeUpdate(b,T)),L=c!=null&&c.type===E&&c.key==null?Ce(c.props.children):c,l=Te(t,Y(L)?L:[L],e,n,r,i,o,a,l,_,d),s.base=e.__e,e.__u&=-161,s.__h.length&&a.push(s),y&&(s.__E=s.__=null)}catch(U){if(a.length=h,e.__v=null,_||o!=null){if(U.then){for(e.__u|=_?160:128;l&&l.nodeType==8&&l.nextSibling;)l=l.nextSibling;o!=null&&(o[o.indexOf(l)]=null),e.__e=l}else if(o!=null)for(ee=o.length;ee--;)ie(o[ee])}else e.__e=n.__e;e.__k==null&&(e.__k=n.__k||[]),U.then||Pe(e),p.__e(U,e,n)}}else o==null&&e.__v==n.__v?(e.__k=n.__k,e.__e=n.__e):l=e.__e=_t(n.__e,e,n,r,i,o,a,_,d);return(c=p.diffed)&&c(e),128&e.__u?void 0:l}function Pe(t){t&&(t.__c&&(t.__c.__e=!0),t.__k&&t.__k.some(Pe))}function Ie(t,e,n){for(var r=0;r<n.length;r++)de(n[r],n[++r],n[++r]);p.__c&&p.__c(e,t),t.some(function(i){try{t=i.__h,i.__h=[],t.some(function(o){o.call(i)})}catch(o){p.__e(o,i.__v)}})}function Ce(t){return typeof t!="object"||t==null||t.__b>0?t:Y(t)?t.map(Ce):t.constructor!==void 0?null:S({},t)}function _t(t,e,n,r,i,o,a,l,_){var d,c,h,s,g,b,T,x=n.props||z,y=e.props,f=e.type;if(f=="svg"?i="http://www.w3.org/2000/svg":f=="math"?i="http://www.w3.org/1998/Math/MathML":i||(i="http://www.w3.org/1999/xhtml"),o!=null){for(d=0;d<o.length;d++)if((g=o[d])&&"setAttribute"in g==!!f&&(f?g.localName==f:g.nodeType==3)){t=g,o[d]=null;break}}if(t==null){if(f==null)return document.createTextNode(y);t=document.createElementNS(i,f,y.is&&y),l&&(p.__m&&p.__m(e,o),l=!1),o=null}if(f==null)x===y||l&&t.data==y||(t.data=y);else{if(o=f=="textarea"&&y.defaultValue!=null?null:o&&q.call(t.childNodes),!l&&o!=null)for(x={},d=0;d<t.attributes.length;d++)x[(g=t.attributes[d]).name]=g.value;for(d in x)g=x[d],d=="dangerouslySetInnerHTML"?h=g:d=="children"||d in y||d=="value"&&"defaultValue"in y||d=="checked"&&"defaultChecked"in y||F(t,d,null,g,i);for(d in y)g=y[d],d=="children"?s=g:d=="dangerouslySetInnerHTML"?c=g:d=="value"?b=g:d=="checked"?T=g:l&&typeof g!="function"||x[d]===g||F(t,d,g,x[d],i);if(c)l||h&&(c.__html==h.__html||c.__html==t.innerHTML)||(t.innerHTML=c.__html),e.__k=[];else if(h&&(t.innerHTML=""),Te(e.type=="template"?t.content:t,Y(s)?s:[s],e,n,r,f=="foreignObject"?"http://www.w3.org/1999/xhtml":i,o,a,o?o[0]:n.__k&&D(n,0),l,_),o!=null)for(d=o.length;d--;)ie(o[d]);l&&f!="textarea"||(d="value",f=="progress"&&b==null?t.removeAttribute("value"):b!=null&&(b!==t[d]||f=="progress"&&!b||f=="option"&&b!=x[d])&&F(t,d,b,x[d],i),d="checked",T!=null&&T!=t[d]&&F(t,d,T,x[d],i))}return t}function de(t,e,n){try{if(typeof t=="function"){var r=typeof t.__u=="function";r&&t.__u(),r&&e==null||(t.__u=t(e))}else t.current=e}catch(i){p.__e(i,n)}}function Se(t,e,n){var r,i;if(p.unmount&&p.unmount(t),(r=t.ref)&&(r.current&&r.current!=t.__e||de(r,null,e)),(r=t.__c)!=null){if(r.componentWillUnmount)try{r.componentWillUnmount()}catch(o){p.__e(o,e)}r.base=r.__P=r.__n=null}if(r=t.__k)for(i=0;i<r.length;i++)r[i]&&Se(r[i],e,n||typeof t.type!="function");n||ie(t.__e),t.__c=t.__=t.__e=void 0}function ut(t,e,n){return this.constructor(t,n)}function Ee(t,e,n){var r,i,o,a;e==document&&(e=document.documentElement),p.__&&p.__(t,e),i=(r=typeof n=="function")?null:n&&n.__k||e.__k,o=[],a=[],se(e,t=(!r&&n||e).__k=st(E,null,[t]),i||z,z,e.namespaceURI,!r&&n?[n]:i?null:e.firstChild?q.call(e.childNodes):null,o,!r&&n?n:i?i.__e:e.firstChild,r,a),Ie(o,t,a),t.props.children=null}function Ne(t){function e(n){var r,i;return this.getChildContext||(r=new Set,(i={})[e.__c]=this,this.getChildContext=function(){return i},this.componentWillUnmount=function(){r=null},this.shouldComponentUpdate=function(o){this.props.value!=o.value&&r.forEach(function(a){a.__e=!0,oe(a)})},this.sub=function(o){r.add(o);var a=o.componentWillUnmount;o.componentWillUnmount=function(){r&&r.delete(o),a&&a.call(o)}}),n.children}return e.__c="__cC"+xe++,e.__=t,e.Provider=e.__l=(e.Consumer=function(n,r){return n.children(r)}).contextType=e,e}q=V.slice,p={__e:function(t,e,n,r){for(var i,o,a;e=e.__;)if((i=e.__c)&&!i.__)try{if((o=i.constructor)&&o.getDerivedStateFromError!=null&&(i.setState(o.getDerivedStateFromError(t)),a=i.__d),i.componentDidCatch!=null&&(i.componentDidCatch(t,r||{}),a=i.__d),a)return i.__E=i}catch(l){t=l}throw t}},me=0,at=function(t){return t!=null&&t.constructor===void 0},j.prototype.setState=function(t,e){var n;n=this.__s!=null&&this.__s!=this.state?this.__s:this.__s=S({},this.state),typeof t=="function"&&(t=t(S({},n),this.props)),t&&S(n,t),t!=null&&this.__v&&(e&&this._sb.push(e),oe(this))},j.prototype.forceUpdate=function(t){this.__v&&(this.__e=!0,t&&this.__h.push(t),oe(this))},j.prototype.render=E,N=[],be=typeof Promise=="function"?Promise.prototype.then.bind(Promise.resolve()):setTimeout,ve=function(t,e){return t.__v.__b-e.__v.__b},K.__r=0,te=Math.random().toString(8),W="__d"+te,$="__a"+te,ye=/(PointerCapture)$|Capture$/i,ae=0,ne=he(!1),re=he(!0),xe=0;var O,m,le,Ae,X=0,Le=[],v=p,De=v.__b,Ue=v.__r,Oe=v.diffed,Re=v.__c,He=v.unmount,$e=v.__;function Q(t,e){v.__h&&v.__h(m,t,X||e),X=0;var n=m.__H||(m.__H={__:[],__h:[]});return t>=n.__.length&&n.__.push({}),n.__[t]}function M(t){return X=1,gt(ze,t)}function gt(t,e,n){var r=Q(O++,2);if(r.t=t,!r.__c&&(r.__=[n?n(e):ze(void 0,e),function(l){var _=r.__N?r.__N[0]:r.__[0],d=r.t(_,l);_!==d&&(r.__N=[d,r.__[1]],r.__c.setState({}))}],r.__c=m,!m.__f)){var i=function(l,_,d){if(!r.__c.__H)return!0;var c=!1,h=r.__c.props!==l;if(r.__c.__H.__.some(function(g){if(g.__N){c=!0;var b=g.__[0];g.__=g.__N,g.__N=void 0,b!==g.__[0]&&(h=!0)}}),o){var s=o.call(this,l,_,d);return c?s||h:s}return!c||h};m.__f=!0;var o=m.shouldComponentUpdate,a=m.componentWillUpdate;m.componentWillUpdate=function(l,_,d){if(this.__e){var c=o;o=void 0,i(l,_,d),o=c}a&&a.call(this,l,_,d)},m.shouldComponentUpdate=i}return r.__N||r.__}function Fe(t,e){var n=Q(O++,4);!v.__s&&je(n.__H,e)&&(n.__=t,n.u=e,m.__h.push(n))}function We(t){return X=5,pt(function(){return{current:t}},[])}function pt(t,e){var n=Q(O++,7);return je(n.__H,e)&&(n.__=t(),n.__H=e,n.__h=t),n.__}function Be(t){var e=m.context[t.__c],n=Q(O++,9);return n.c=t,e?(n.__==null&&(n.__=!0,e.sub(m)),e.props.value):t.__}function ft(){for(var t;t=Le.shift();){var e=t.__H;if(t.__P&&e)try{e.__h.some(G),e.__h.some(ce),e.__h=[]}catch(n){e.__h=[],v.__e(n,t.__v)}}}v.__b=function(t){m=null,De&&De(t)},v.__=function(t,e){t&&e.__k&&e.__k.__m&&(t.__m=e.__k.__m),$e&&$e(t,e)},v.__r=function(t){Ue&&Ue(t),O=0;var e=(m=t.__c).__H;e&&(le===m?(e.__h=[],m.__h=[],e.__.some(function(n){n.__N&&(n.__=n.__N),n.u=n.__N=void 0})):(e.__h.some(G),e.__h.some(ce),e.__h=[],O=0)),le=m},v.diffed=function(t){Oe&&Oe(t);var e=t.__c;e&&e.__H&&(e.__H.__h.length&&(Le.push(e)!==1&&Ae===v.requestAnimationFrame||((Ae=v.requestAnimationFrame)||ht)(ft)),e.__H.__.some(function(n){n.u&&(n.__H=n.u,n.u=void 0)})),le=m=null},v.__c=function(t,e){e.some(function(n){try{n.__h.some(G),n.__h=n.__h.filter(function(r){return!r.__||ce(r)})}catch(r){e.some(function(i){i.__h&&(i.__h=[])}),e=[],v.__e(r,n.__v)}}),Re&&Re(t,e)},v.unmount=function(t){He&&He(t);var e,n=t.__c;n&&n.__H&&(n.__H.__.some(function(r){try{G(r)}catch(i){e=i}}),n.__H=void 0,e&&v.__e(e,n.__v))};var Me=typeof requestAnimationFrame=="function";function ht(t){var e,n=function(){clearTimeout(r),Me&&cancelAnimationFrame(e),setTimeout(t)},r=setTimeout(n,35);Me&&(e=requestAnimationFrame(n))}function G(t){var e=m,n=t.__c;typeof n=="function"&&(t.__c=void 0,n()),m=e}function ce(t){var e=m;t.__c=t.__(),m=e}function je(t,e){return!t||t.length!==e.length||e.some(function(n,r){return n!==t[r]})}function ze(t,e){return typeof e=="function"?e(t):e}var Ve="(prefers-color-scheme: dark)";function mt(t){let e=/rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)/.exec(t||"");return e?{r:+e[1],g:+e[2],b:+e[3],a:e[4]==null?1:+e[4]}:null}function bt(t){let e=t;for(;e;){let n=mt(getComputedStyle(e).backgroundColor);if(n&&n.a>0)return .2126*n.r+.7152*n.g+.0722*n.b<128?"dark":"light";e=e.parentElement}return window.matchMedia&&window.matchMedia(Ve).matches?"dark":"light"}function Ke(t){let[e,n]=M("light");return Fe(()=>{let r=t.current;if(!r)return;let i=()=>n(bt(r.parentElement||r));i();let o=new MutationObserver(i),a={attributes:!0,attributeFilter:["class","style","data-theme","data-color-scheme"]};o.observe(document.documentElement,a),document.body&&o.observe(document.body,a);let l=window.matchMedia?window.matchMedia(Ve):null;return l&&l.addEventListener("change",i),()=>{o.disconnect(),l&&l.removeEventListener("change",i)}},[t]),e}var w={boxRadius:8,boxStroke:1.5,outlineRadius:10,outlineStroke:2,outlineDash:"6 4",edgeStroke:1.5,edgeStrokeActive:3,edgeDash:"5 4",badgeRadius:13,badgeRadiusActive:15,badgeOnEdgeMin:60},vt=`
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
`,qe=`
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
`,yt=`
  .dg-diagram { ${vt} }
  @media (prefers-color-scheme: dark) {
    .dg-diagram:not([data-dg-theme="light"]) { ${qe} }
  }
  .dg-diagram[data-dg-theme="dark"] { ${qe} }

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
`,Ye="dg-diagram-styles";function Ge(){if(typeof document=="undefined"||document.getElementById(Ye))return;let t=document.createElement("style");t.id=Ye,t.textContent=yt,document.head.appendChild(t)}var xt=0,Rt=Array.isArray;function u(t,e,n,r,i,o){e||(e={});var a,l,_=e;if("ref"in _)for(l in _={},e)l=="ref"?a=e[l]:_[l]=e[l];var d={type:t,props:_,key:n,ref:a,__k:null,__:null,__b:0,__e:null,__c:null,constructor:void 0,__v:--xt,__i:-1,__u:0,__source:i,__self:o};if(typeof t=="function"&&(a=t.defaultProps))for(l in a)_[l]===void 0&&(_[l]=a[l]);return p.vnode&&p.vnode(d),d}var Qe=Ne({markerPrefix:"dg-arrow"});function Tt(t){return t.map(([e,n])=>`${e},${n}`).join(" ")}function wt(t,e="Edge"){for(let n=1;n<t.length;n++){let[r,i]=t[n-1],[o,a]=t[n];if(r!==o&&i!==a)throw new Error(`${e}: diagonal segment (${r},${i})\u2192(${o},${a}); connectors must be horizontal/vertical \u2014 add a bend point`)}return t}function J({x:t,y:e,w:n,h:r,label:i,variant:o="neutral",filled:a}){return u("g",{class:`dg-outline dg-outline-${o}${a?" dg-outline-filled":""}`,children:[u("rect",{x:t,y:e,width:n,height:r,rx:w.outlineRadius,"stroke-width":w.outlineStroke,"stroke-dasharray":w.outlineDash}),i?u("text",{x:t+14,y:e+22,class:"dg-label dg-label-outline",children:i}):null]})}function k({x:t,y:e,w:n,h:r,label:i,sub:o,variant:a="default",small:l}){let _=a.startsWith("solid-"),d=o?e+r/2-6:e+r/2+5;return u("g",{class:`dg-box dg-box-${a}${_?" dg-box-solid":""}`,children:[u("rect",{x:t,y:e,width:n,height:r,rx:w.boxRadius,"stroke-width":w.boxStroke}),u("text",{x:t+n/2,y:d,"text-anchor":"middle",class:l?"dg-label dg-label-sm":"dg-label dg-label-bold",children:i}),o?u("text",{x:t+n/2,y:d+(l?14:18),"text-anchor":"middle",class:"dg-label dg-label-sub",children:o}):null]})}function _e({points:t,dashed:e,active:n,dir:r,id:i}){wt(t,i?`Edge ${i}`:"Edge");let{markerPrefix:o}=Be(Qe),a=`${o}-${n?"active":"default"}`,l=r&&r!=="none";return u("polyline",{class:`dg-edge${n?" dg-edge-active":""}`,points:Tt(t),fill:"none","stroke-width":n?w.edgeStrokeActive:w.edgeStroke,"stroke-dasharray":e?w.edgeDash:void 0,"marker-end":l?`url(#${a})`:void 0,"marker-start":r==="both"?`url(#${a})`:void 0})}function Je({id:t,label:e,name:n,x:r,y:i,active:o,pressed:a,onSelect:l,onHover:_}){let d=(c,h)=>{c.pointerType==="mouse"&&_&&_(h)};return u("g",{class:`dg-badge${o?" dg-badge-active":""}`,role:"button",tabindex:"0","aria-pressed":a,"aria-label":n?`${t}: ${n}`:t,onClick:()=>l(t),onPointerEnter:c=>d(c,t),onPointerLeave:c=>d(c,null),onKeyDown:c=>{(c.key==="Enter"||c.key===" ")&&(c.preventDefault(),l(t))},children:[u("circle",{cx:r,cy:i,r:o?w.badgeRadiusActive:w.badgeRadius,"stroke-width":"1.5"}),u("text",{x:r,y:i+4,"text-anchor":"middle",class:"dg-badge-label",children:e!=null?e:t})]})}function Ze(){let[t,e]=M(null),[n,r]=M(null);return{shownId:n||t,pinnedId:t,select:o=>e(a=>a===o?null:o),setHover:r}}function Pt({tone:t="neutral",id:e,text:n}){return u("span",{class:`dg-chip dg-chip-${t}`,children:[u("span",{class:"dg-chip-id",children:e}),n?u("span",{class:"dg-chip-desc",children:n}):null]})}function et({id:t,title:e,desc:n,chips:r,hint:i,children:o}){return u("div",{class:"dg-panel","aria-live":"polite",children:e?u(E,{children:[u("div",{class:"dg-panel-title",children:[t?u(E,{children:[t," \u2014 "]}):null,e]}),n?u("div",{class:"dg-panel-desc",children:n}):null,r&&r.length?u("div",{class:"dg-chips",children:r.map(a=>u(Pt,{...a},`${a.tone}-${a.id}`))}):null,o]}):u("div",{class:"dg-hint",children:i})})}function Xe({id:t,active:e}){return u("marker",{id:t,viewBox:"0 0 10 10",refX:"8",refY:"5",markerWidth:"6",markerHeight:"6",orient:"auto-start-reverse",children:u("path",{d:"M0,0 L10,5 L0,10 z",class:`dg-marker${e?" dg-marker-active":""}`})})}function tt({id:t,viewH:e,viewW:n=960,label:r,footer:i,children:o}){Ge();let a=We(null),l=Ke(a),_=`dg-arrow-${t}`;return u("div",{class:"dg-diagram","data-dg-theme":l,ref:a,children:[u(Qe.Provider,{value:{markerPrefix:_},children:u("svg",{viewBox:`0 0 ${n} ${e}`,role:"group","aria-label":r,children:[u("defs",{children:[u(Xe,{id:`${_}-default`}),u(Xe,{id:`${_}-active`,active:!0})]}),o]})}),i]})}var Z={"UC-1":"Provision","UC-2":"Authenticate","UC-3":"Task exchange","IP-1":"Operator I/O","IP-2":"Docker control plane","IP-3":"Exec channel","IP-4":"Kernel boundary","IP-4a":"Container \u2192 Sentry","IP-4b":"Sentry \u2192 host kernel","IP-5":"Container filesystem","IP-6":"Network egress",T1a:"outbound POST",T1b:"DNS tunnelling",T1d:"ambient credential",T1e:"terminal query \u2192 stdin","T2a\u2013d":"supply chain",T2c:"install-time hooks",T3b:"irreversible remote action",T3c:"destructive retry loop","T4a\u2013c":"lateral movement",T5b:"PATH / build-file backdoor",T5d:"reused sandbox state",T6a:"shared kernel surface",T6b:"runc binary overwrite",T6c:"leaked-fd escape",T6d:"privileged / misconfigured",T6e:"shared-hardware side channels",T6f:"no cgroup caps","T6g\u2013j":"Sentry / Gofer residual",T6k:"platform exposure",T6l:"env flips runtime","T7a\u2013c":"no PID / mem / disk caps",T7d:"Sentry DoS",T7e:"unbounded read_until \xB7 no exec timeout",T8b:"unscoped docker.from_env()",T9a:"marker-driven state",T9b:"unsupervised tool loop",T9c:"no per-action gate",T9d:"raw print() to terminal",T9e:"CLI output is the prompt"},nt={"IP-1":{name:"Operator I/O",code:"gateway.ask_user() \u2192 interrupt(), answered by input() in main.py; gateway.notify_user() \u2192 print()",desc:"The sole human-in-the-loop gate, at turn granularity only. Agent text reaches the terminal emulator unfiltered \u2014 the _clean regex runs on login output alone.",uc:["UC-1","UC-2","UC-3"],open:["T9c","T9d","T9e","T1e"],closed:[]},"IP-2":{name:"Docker control plane",code:"container.client() \u2014 module-level docker.from_env(); create(), find(), containers.get() in nodes 2\u20134, 6",desc:"Full daemon access from the harness process. No socket is mounted into the container and the agent cannot spawn containers, so only the orchestrator side is live.",uc:["UC-1","UC-2","UC-3"],open:["T8b","T5d"],closed:[]},"IP-3":{name:"Exec channel",code:"ExecSession (exec_create stdin+tty, attached socket) for the login relay; ct.exec() \u2192 exec_run(demux=False) for auth status and claude -p",desc:"Free text in both directions. Login transitions on literal markers; read_until grows its buffer unbounded; the timeout ct.exec() accepts is never passed on.",uc:["UC-2","UC-3"],open:["T9a","T9b","T7e","T3c"],closed:[]},"IP-5":{name:"Container filesystem",code:"create(): read_only=False; no volumes, mounts, or tmpfs",desc:"The whole rootfs is writable \u2014 including the CLI binary on PATH and the OAuth session claude auth login persists \u2014 and node 2 reuses it across runs. No host path is mounted, so the blast radius stops at the container.",uc:["UC-3"],open:["T1d","T2c","T5b","T5d","T7a\u2013c"],closed:[]},"IP-6":{name:"Network egress",code:'create(): network_mode="bridge"; no egress or DNS policy',desc:"The CLI reaches the Anthropic API because everything is reachable: registries, link-local metadata, host services, and every other agent container on the same default bridge.",uc:["UC-2","UC-3"],open:["T1a","T1b","T2a\u2013d","T4a\u2013c","T3b"],closed:[]}},rt={runc:{...nt,"IP-4":{name:"Kernel boundary",code:'CONTAINER_RUNTIME=runc in the environment; cap_drop=["ALL"], security_opt=["no-new-privileges"] still apply',desc:"Raw syscalls to the shared host kernel. cap_drop and no mounts still close the misconfiguration class, but every kernel and runc CVE is one hop from the orchestrator.",uc:["UC-1","UC-3"],open:["T6a","T6b","T6c","T6e","T6f","T6l"],closed:["T6d"]}},runsc:{...nt,"IP-4a":{name:"Container \u2192 Sentry",code:'create(): runtime=config.CONTAINER_RUNTIME (os.getenv default "runsc"), cap_drop=["ALL"], security_opt=["no-new-privileges"]',desc:"Syscalls are intercepted by gVisor\u2019s userspace Sentry before they would ever reach the host kernel. Closes the shared-kernel and runc-CVE cases; the kwargs close the misconfiguration class.",uc:["UC-1","UC-3"],open:["T6g\u2013j","T6l"],closed:["T6a","T6b","T6c","T6d"]},"IP-4b":{name:"Sentry \u2192 host kernel",code:"whatever the daemon\u2019s runsc platform is (ptrace / KVM / systrap); no mem_limit, pids_limit, or CPU quota",desc:"The Sentry\u2019s own narrow, seccomp-filtered call set. A Sentry compromise lands here, contained to that process \u2014 but nothing caps what it may consume.",uc:["UC-1","UC-3"],open:["T6e","T6f","T6k","T7a\u2013c","T7d"],closed:[]}}};var P={operator:{x:20,y:20,w:120,h:56},terminal:{x:180,y:20,w:200,h:56},orchestrator:{x:50,y:140,w:220,h:80},engine:{x:340,y:140,w:180,h:80},cli:{x:370,y:290,w:290,h:70},login:{x:370,y:400,w:120,h:60},rootfs:{x:540,y:400,w:120,h:60},sentry:{x:340,y:500,w:350,h:56},api:{x:770,y:140,w:150,h:56},anything:{x:770,y:230,w:150,h:56},siblings:{x:770,y:320,w:150,h:56}};function It(t){let e=t==="runc",n=e?520:600,r=e?520:600;return{isRunc:e,viewH:n+100,kernel:{x:50,y:n,w:640,h:70},outlines:{host:{x:20,y:100,w:700,h:r},container:{x:340,y:250,w:350,h:230},outside:{x:750,y:100,w:190,h:r}},edges:[{id:"IP-1",points:[[280,76],[280,108],[160,108],[160,140]],badge:[220,108],dir:"both"},{id:"IP-2",points:[[270,180],[340,180]],badge:[305,180],dir:"end"},{id:"IP-3",points:[[160,220],[160,325],[370,325]],badge:[265,325],dir:"both"},...e?[{id:"IP-4",points:[[515,360],[515,n]],badge:[515,440],dir:"end"}]:[{id:"IP-4a",points:[[515,360],[515,500]],badge:[515,430],dir:"end"},{id:"IP-4b",points:[[515,556],[515,n]],badge:[545,578],dir:"end"}],{id:"IP-5",points:[[600,360],[600,400]],badge:[630,380],dir:"both"},{id:"IP-6",points:[[660,325],[740,325]],badge:[705,325],dir:"both"}],plain:[{points:[[140,48],[180,48]],dir:"both"},{points:[[430,220],[430,250]],dir:"end"},{points:[[430,360],[430,400]],dir:"end"},{points:[[80,220],[80,n]],dashed:!0},{points:[[740,168],[740,348]]},{points:[[740,168],[770,168]],dir:"end"},{points:[[740,258],[770,258]],dir:"end"},{points:[[740,348],[770,348]],dir:"end"}]}}function ue({id:t="boundary",runtime:e="runsc"}){let{shownId:n,pinnedId:r,select:i,setHover:o}=Ze(),a=It(e),l=rt[e],_=n?l[n]:null,d=_?[..._.uc.map(c=>({tone:"info",id:c,text:Z[c]})),..._.closed.map(c=>({tone:"safe",id:c,text:Z[c]})),..._.open.map(c=>({tone:"danger",id:c,text:Z[c]}))]:[];return u(tt,{id:t,viewH:a.viewH,label:`System block diagram of the minimal harness under runtime=${e} with its interaction points`,footer:u(et,{id:n,title:_==null?void 0:_.name,desc:_?`${_.code}. ${_.desc}`:void 0,chips:d,hint:"Hover or click a numbered badge for where that interaction point lives in the code, the use cases on it, and the threat cases it closes (green) or leaves open (red)."}),children:[u(J,{...a.outlines.host,label:"HOST SYSTEM",variant:a.isRunc?"danger":"neutral"}),u(J,{...a.outlines.container,label:"AGENT CONTAINER",variant:a.isRunc?"danger":"safe",filled:!0}),u(J,{...a.outlines.outside,label:"BRIDGE NETWORK",variant:"danger"}),u(k,{...P.operator,label:"Operator",sub:"(human)"}),u(k,{...P.terminal,label:"Terminal emulator",sub:"input() \xB7 print()"}),u(k,{...P.orchestrator,label:"Host process",sub:"main.py \u2192 graph.py \xB7 MemorySaver"}),u(k,{...P.engine,label:"Docker Engine",sub:"docker.from_env()"}),u(k,{...P.cli,label:"claude CLI",sub:"auth login \xB7 -p --output-format text"}),u(k,{...P.login,label:"Login state",sub:"OAuth session",small:!0}),u(k,{...P.rootfs,label:"Rootfs",sub:"read_only=False",small:!0}),a.isRunc?null:u(k,{...P.sentry,label:"gVisor Sentry",sub:"runtime=runsc \xB7 userspace kernel",variant:"solid-safe"}),u(k,{...a.kernel,label:"HOST KERNEL",sub:a.isRunc?"shared \u2014 one hop from the container":"shared by host process, engine and Sentry",variant:a.isRunc?"solid-danger":"solid-dark"}),u(k,{...P.api,label:"Anthropic API",sub:"OAuth + inference",small:!0}),u(k,{...P.anything,label:"Anything else",sub:"registries \xB7 IMDS \xB7 host",small:!0}),u(k,{...P.siblings,label:"Sibling sandboxes",sub:"same default bridge",small:!0}),a.plain.map((c,h)=>u(_e,{...c},h)),a.edges.map(c=>u(_e,{id:c.id,points:c.points,dir:c.dir,active:n===c.id},c.id)),a.edges.map(c=>u(Je,{id:c.id,label:c.id.replace("IP-",""),name:l[c.id].name,x:c.badge[0],y:c.badge[1],active:n===c.id,pressed:r===c.id,onSelect:i,onHover:o},c.id))]})}var Ct={"island-boundary":{id:"boundary-runsc",runtime:"runsc"},"boundary-diagram-runc":{id:"boundary-runc",runtime:"runc"}};function ot(){for(let[t,e]of Object.entries(Ct)){let n=document.getElementById(t);n&&Ee(u(ue,{...e}),n)}}document.readyState==="loading"?document.addEventListener("DOMContentLoaded",ot):ot();})();
