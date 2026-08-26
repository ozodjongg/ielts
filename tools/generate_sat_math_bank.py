#!/usr/bin/env python3
from __future__ import annotations
import csv, json, math, random, uuid
from dataclasses import dataclass
from fractions import Fraction
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'data' / 'sat-math-bank'
OUT.mkdir(parents=True, exist_ok=True)
NS = uuid.UUID('e75aca8e-6c74-4d84-a866-7ad8f5c8f564')

@dataclass(frozen=True)
class Topic:
    code: str
    domain: str
    name: str
    difficulty: int
    points: int

TOPICS = [
    # Algebra
    Topic('ALG01','Algebra','Linear equations in one variable',2,1),
    Topic('ALG02','Algebra','Linear equations with fractions',3,1),
    Topic('ALG03','Algebra','Linear inequalities',3,1),
    Topic('ALG04','Algebra','Systems of two linear equations',4,2),
    Topic('ALG05','Algebra','Systems in word problems',5,2),
    Topic('ALG06','Algebra','Slope from two points',2,1),
    Topic('ALG07','Algebra','Equation of a line',3,1),
    Topic('ALG08','Algebra','Parallel and perpendicular lines',4,2),
    Topic('ALG09','Algebra','Intercepts of linear equations',3,1),
    Topic('ALG10','Algebra','Evaluating linear functions',2,1),
    Topic('ALG11','Algebra','Rates of change',3,1),
    Topic('ALG12','Algebra','Linear percent relationships',4,2),
    Topic('ALG13','Algebra','Absolute value equations',4,2),
    Topic('ALG14','Algebra','No solution and infinitely many solutions',5,2),
    Topic('ALG15','Algebra','Parameters in linear equations',5,2),
    Topic('ALG16','Algebra','Direct variation',3,1),
    Topic('ALG17','Algebra','Linear unit conversions',3,1),
    Topic('ALG18','Algebra','Mixture equations',5,2),
    Topic('ALG19','Algebra','Age word problems',4,2),
    Topic('ALG20','Algebra','Distance, rate, and time',4,2),
    # Advanced Math
    Topic('ADV01','Advanced Math','Roots of quadratic equations',5,2),
    Topic('ADV02','Advanced Math','Vertex of a parabola',5,2),
    Topic('ADV03','Advanced Math','Factoring quadratics',5,2),
    Topic('ADV04','Advanced Math','Quadratic discriminant',6,3),
    Topic('ADV05','Advanced Math','Polynomial evaluation',4,2),
    Topic('ADV06','Advanced Math','Polynomial remainder theorem',6,3),
    Topic('ADV07','Advanced Math','Rational equations',6,3),
    Topic('ADV08','Advanced Math','Simplifying rational expressions',5,2),
    Topic('ADV09','Advanced Math','Exponent rules',4,2),
    Topic('ADV10','Advanced Math','Exponential growth',5,2),
    Topic('ADV11','Advanced Math','Exponential decay',5,2),
    Topic('ADV12','Advanced Math','Radical equations',6,3),
    Topic('ADV13','Advanced Math','Function composition',5,2),
    Topic('ADV14','Advanced Math','Inverse functions',5,2),
    Topic('ADV15','Advanced Math','Linear-quadratic systems',7,3),
    Topic('ADV16','Advanced Math','Complex numbers',6,3),
    Topic('ADV17','Advanced Math','Polynomial degree and leading terms',5,2),
    Topic('ADV18','Advanced Math','Function transformations',5,2),
    Topic('ADV19','Advanced Math','Equations in equivalent forms',6,3),
    Topic('ADV20','Advanced Math','Equivalent algebraic expressions',6,3),
    # Problem Solving and Data Analysis
    Topic('PSDA01','Problem Solving and Data Analysis','Ratios',2,1),
    Topic('PSDA02','Problem Solving and Data Analysis','Proportions',3,1),
    Topic('PSDA03','Problem Solving and Data Analysis','Percent change',3,1),
    Topic('PSDA04','Problem Solving and Data Analysis','Weighted averages',5,2),
    Topic('PSDA05','Problem Solving and Data Analysis','Mean and median',3,1),
    Topic('PSDA06','Problem Solving and Data Analysis','Range and spread',3,1),
    Topic('PSDA07','Problem Solving and Data Analysis','Basic probability',3,1),
    Topic('PSDA08','Problem Solving and Data Analysis','Conditional probability',6,3),
    Topic('PSDA09','Problem Solving and Data Analysis','Two-way tables',5,2),
    Topic('PSDA10','Problem Solving and Data Analysis','Interpreting scatterplots',4,2),
    Topic('PSDA11','Problem Solving and Data Analysis','Linear model predictions',4,2),
    Topic('PSDA12','Problem Solving and Data Analysis','Sampling and inference',6,3),
    Topic('PSDA13','Problem Solving and Data Analysis','Margin of error',6,3),
    Topic('PSDA14','Problem Solving and Data Analysis','Unit rates',3,1),
    Topic('PSDA15','Problem Solving and Data Analysis','Density and derived units',4,2),
    Topic('PSDA16','Problem Solving and Data Analysis','Unit conversion in context',4,2),
    Topic('PSDA17','Problem Solving and Data Analysis','Percentages from data tables',4,2),
    Topic('PSDA18','Problem Solving and Data Analysis','Exponential models in context',6,3),
    Topic('PSDA19','Problem Solving and Data Analysis','Quartiles and medians',5,2),
    Topic('PSDA20','Problem Solving and Data Analysis','Comparing variability',5,2),
    # Geometry and Trigonometry
    Topic('GEO01','Geometry and Trigonometry','Area of triangles',2,1),
    Topic('GEO02','Geometry and Trigonometry','Area and perimeter of rectangles',2,1),
    Topic('GEO03','Geometry and Trigonometry','Area of circles',3,1),
    Topic('GEO04','Geometry and Trigonometry','Circumference of circles',3,1),
    Topic('GEO05','Geometry and Trigonometry','Pythagorean theorem',3,1),
    Topic('GEO06','Geometry and Trigonometry','Similar triangles',5,2),
    Topic('GEO07','Geometry and Trigonometry','Angles with parallel lines',4,2),
    Topic('GEO08','Geometry and Trigonometry','Interior angles of polygons',4,2),
    Topic('GEO09','Geometry and Trigonometry','Regular polygons',5,2),
    Topic('GEO10','Geometry and Trigonometry','Volume of rectangular prisms',3,1),
    Topic('GEO11','Geometry and Trigonometry','Volume of cylinders',4,2),
    Topic('GEO12','Geometry and Trigonometry','Volume of spheres',5,2),
    Topic('GEO13','Geometry and Trigonometry','Distance in the coordinate plane',4,2),
    Topic('GEO14','Geometry and Trigonometry','Midpoints in the coordinate plane',3,1),
    Topic('GEO15','Geometry and Trigonometry','Perpendicular slopes',4,2),
    Topic('GEO16','Geometry and Trigonometry','Right-triangle trigonometry',6,3),
    Topic('GEO17','Geometry and Trigonometry','Special right triangles',6,3),
    Topic('GEO18','Geometry and Trigonometry','Arc length',6,3),
    Topic('GEO19','Geometry and Trigonometry','Sector area',6,3),
    Topic('GEO20','Geometry and Trigonometry','Equations of circles',6,3),
]


def fmt(x):
    if isinstance(x, Fraction):
        if x.denominator == 1: return str(x.numerator)
        return f'{x.numerator}/{x.denominator}'
    if isinstance(x, float):
        s=f'{x:.2f}'.rstrip('0').rstrip('.')
        return s
    return str(x)


def options(correct, distractors, rng):
    vals=[]
    for v in [correct]+list(distractors):
        s=fmt(v)
        if s not in vals: vals.append(s)
    bump=1
    while len(vals)<4:
        try:
            base=float(fmt(correct))
            candidate=fmt(base+bump)
        except Exception:
            candidate=str(bump)
        if candidate not in vals: vals.append(candidate)
        bump += 1
    vals=vals[:4]
    rng.shuffle(vals)
    return vals, vals.index(fmt(correct))


def q_algebra(i, rng):
    k=i+1
    if k==1:
        x=rng.randint(-12,18); a=rng.choice([2,3,4,5,6,7,8]); b=rng.randint(-20,20); c=a*x+b
        return f'Solve for x: {a}x {b:+d} = {c}.', x, [x+1,x-1,-x], f'Subtract {b} from both sides and divide by {a}; x = {x}.'
    if k==2:
        x=rng.randint(-10,15); d=rng.choice([2,3,4,5]); a=rng.choice([1,2,3]); b=rng.randint(-8,8); c=Fraction(a*x,d)+b
        return f'Solve for x: ({a}x)/{d} {b:+d} = {fmt(c)}.', x, [x+d,x-d,-x], f'Isolate ({a}x)/{d}, then multiply by {d}/{a}. The solution is x = {x}.'
    if k==3:
        x0=rng.randint(-5,10); a=rng.randint(2,8); b=rng.randint(-10,10); c=a*x0+b+rng.randint(1,8)
        bound=Fraction(c-b,a)
        return f'Which value is the greatest integer x that satisfies {a}x {b:+d} < {c}?', math.ceil(float(bound))-1, [math.floor(float(bound)), math.ceil(float(bound)), math.ceil(float(bound))+1], f'x < {fmt(bound)}, so the greatest integer solution is {math.ceil(float(bound))-1}.'
    if k==4:
        x=rng.randint(-8,10); y=rng.randint(-8,10); a,b=rng.sample([1,2,3,4,5],2); c=a*x+b*y; d,e=rng.sample([1,2,3,4,5,6],2)
        while a*e==b*d: d,e=rng.sample([1,2,3,4,5,6],2)
        f=d*x+e*y
        return f'The system {a}x + {b}y = {c} and {d}x + {e}y = {f} has solution (x, y). What is x?', x, [y,x+1,x-1], f'Solving the two equations simultaneously gives (x, y)=({x}, {y}).'
    if k==5:
        adult=rng.randint(8,30); child=rng.randint(5,adult-1); na=rng.randint(10,40); nc=rng.randint(10,40); total=adult*na+child*nc; n=na+nc
        return f'A theater sold {n} tickets. Adult tickets cost ${adult} and child tickets cost ${child}. Total revenue was ${total}. How many adult tickets were sold?', na, [nc,na+5,max(1,na-5)], f'Let a be adult tickets. {adult}a + {child}({n}-a) = {total}, which gives a = {na}.'
    if k==6:
        m=rng.choice([-5,-4,-3,-2,2,3,4,5]); x1=rng.randint(-5,5); y1=rng.randint(-10,10); dx=rng.choice([1,2,3,4]); x2=x1+dx; y2=y1+m*dx
        return f'What is the slope of the line through ({x1}, {y1}) and ({x2}, {y2})?', m, [m+1,m-1,-m], f'Slope = ({y2}-{y1})/({x2}-{x1}) = {m}.'
    if k==7:
        m=rng.randint(-5,5) or 2; b=rng.randint(-10,10); x=rng.randint(-4,6); y=m*x+b
        return f'A line has slope {m} and passes through ({x}, {y}). What is its y-intercept?', b, [m,y,b+1], f'Using y=mx+b, b={y}-{m}({x})={b}.'
    if k==8:
        m=Fraction(rng.choice([1,2,3,4,5]),rng.choice([1,2,3,4])); perp=-1/m
        return f'A line has slope {fmt(m)}. What is the slope of a line perpendicular to it?', perp, [m,-m,1/m], f'Perpendicular slopes have product -1, so the slope is {fmt(perp)}.'
    if k==9:
        a=rng.randint(2,8); xint=rng.randint(-8,8) or 3; c=a*xint
        return f'The line {a}x + {rng.randint(2,7)}y = {c} crosses the x-axis at (p, 0). What is p?', xint, [a,c,xint+1], f'Set y=0: {a}p={c}, so p={xint}.'
    if k==10:
        m=rng.randint(-6,6) or 3; b=rng.randint(-15,15); x=rng.randint(-8,8); ans=m*x+b
        return f'For f(x) = {m}x {b:+d}, what is f({x})?', ans, [ans+m,ans-m,x], f'Substitute x={x}: f({x})={m}({x}){b:+d}={ans}.'
    if k==11:
        start=rng.randint(20,80); rate=rng.randint(3,15); t=rng.randint(4,12); ans=start+rate*t
        return f'A tank contains {start} liters and is filled at a constant rate of {rate} liters per minute. How many liters are in the tank after {t} minutes?', ans, [rate*t,ans-rate,ans+rate], f'Amount = initial + rate×time = {start}+{rate}×{t}={ans}.'
    if k==12:
        base=rng.choice([40,50,60,80,100,120]); pct=rng.choice([10,15,20,25,30]); units=rng.randint(2,9); ans=base*(100+pct)*units//100
        return f'A service costs ${base} per unit and its price increases by {pct}%. What is the new cost of {units} units?', ans, [base*units,ans-base,ans+base], f'New unit price is {base}×{100+pct}/100, then multiply by {units}; total ${ans}.'
    if k==13:
        center=rng.randint(-8,8); dist=rng.randint(2,10); target=rng.choice([center+dist,center-dist])
        return f'If |x {(-center):+d}| = {dist}, which of the following could be x?', target, [center,center+dist+1,center-dist-1], f'|x-{center}|={dist} gives x={center}+{dist} or x={center}-{dist}.'
    if k==14:
        a=rng.randint(2,7); b=rng.randint(-10,10); scale=rng.randint(2,5); mode=rng.choice(['none','infinite'])
        if mode=='infinite': c=a*scale; d=b*scale; ans='infinitely many solutions'; ds=['no solution','exactly one solution','exactly two solutions']
        else: c=a*scale; d=b*scale+rng.choice([1,2,3]); ans='no solution'; ds=['infinitely many solutions','exactly one solution','exactly two solutions']
        return f'How many solutions does the system y = {a}x {b:+d} and {scale}y = {c}x {d:+d} have?', ans, ds, f'The equations have the same slope after simplification; their intercept relationship makes the system have {ans}.'
    if k==15:
        x=rng.randint(2,10); a=rng.randint(2,7); c=rng.randint(-10,10); target=rng.randint(-8,8); b=target; rhs=(a+b)*x+c
        return f'For what value of k does ({a}+k)x {c:+d} = {rhs} have solution x={x}?', b, [b+1,b-1,a], f'Substitute x={x} and solve ({a}+k){x}{c:+d}={rhs}; k={b}.'
    if k==16:
        kk=rng.randint(2,12); x=rng.randint(2,10); y=kk*x
        return f'y varies directly with x. If y={y} when x={x}, what is y when x={x+3}?', kk*(x+3), [y+3,kk+3,y+kk], f'Direct variation gives y=kx with k={y}/{x}={kk}. Thus y={kk}({x+3})={kk*(x+3)}.'
    if k==17:
        km=rng.randint(3,20); meters=km*1000
        return f'A route is {km} kilometers long. How many meters long is the route?', meters, [km*100,km*10000,meters+1000], f'1 kilometer=1000 meters, so {km} km={meters} m.'
    if k==18:
        c1=rng.choice([10,20,30]); c2=rng.choice([50,60,70]); total=rng.choice([20,30,40]); x=rng.randint(5,total-5); target=Fraction(c1*x+c2*(total-x),total)
        return f'A chemist mixes x liters of a {c1}% solution with {total}-x liters of a {c2}% solution to obtain a {fmt(target)}% mixture. What is x?', x, [total-x,x+2,max(1,x-2)], f'Solve {c1}x+{c2}({total}-x)={fmt(target)}({total}); x={x}.'
    if k==19:
        diff=rng.randint(4,20); young=rng.randint(8,30); old=young+diff; years=rng.randint(2,8); s=(young+years)+(old+years)
        return f'Two people differ in age by {diff} years. In {years} years, the sum of their ages will be {s}. What is the younger person’s current age?', young, [old,young+years,max(1,young-years)], f'Let y be the younger age. y+(y+{diff})+2({years})={s}, so y={young}.'
    # 20
    speed=rng.randint(35,75); hours=rng.randint(2,6); dist=speed*hours
    return f'A car travels {dist} miles at a constant speed of {speed} miles per hour. How many hours does the trip take?', hours, [hours+1,max(1,hours-1),speed], f'Time=distance/rate={dist}/{speed}={hours} hours.'


def q_advanced(i, rng):
    k=i+1
    if k==1:
        r1=rng.randint(-8,8); r2=rng.randint(-8,8)
        while r2==r1: r2=rng.randint(-8,8)
        b=-(r1+r2); c=r1*r2
        return f'One solution of x² {b:+d}x {c:+d} = 0 is {r1}. What is the other solution?', r2, [r1,-r2,r2+1], f'The quadratic factors as (x-{r1})(x-{r2})=0, so the other root is {r2}.'
    if k==2:
        h=rng.randint(-6,6); kk=rng.randint(-10,10); a=rng.choice([1,2,-1,-2])
        return f'The graph of y = {a}(x {(-h):+d})² {kk:+d} has vertex (h, k). What is h?', h, [kk,-h,h+1], f'In vertex form y=a(x-h)²+k, the vertex is ({h},{kk}).'
    if k==3:
        p=rng.randint(-9,9); q=rng.randint(-9,9); b=-(p+q); c=p*q; ans=f'(x {(-p):+d})(x {(-q):+d})'
        ds=[f'(x {p:+d})(x {q:+d})',f'(x {(-p):+d})(x {q:+d})',f'(x {p:+d})(x {(-q):+d})']
        return f'Which expression is equivalent to x² {b:+d}x {c:+d}?', ans, ds, f'Numbers {-p} and {-q} give the required sum and product, so the factorization is {ans}.'
    if k==4:
        a=rng.randint(1,4); b=rng.randint(-10,10); c=rng.randint(-10,10); disc=b*b-4*a*c
        return f'What is the discriminant of {a}x² {b:+d}x {c:+d} = 0?', disc, [b*b+4*a*c,disc+4*a,disc-4*a], f'Discriminant b²-4ac={b}²-4({a})({c})={disc}.'
    if k==5:
        a=rng.randint(-4,4) or 2; b=rng.randint(-6,6); c=rng.randint(-8,8); x=rng.randint(-4,4); ans=a*x*x+b*x+c
        return f'If p(x) = {a}x² {b:+d}x {c:+d}, what is p({x})?', ans, [ans+a,ans+b,ans+1], f'Substitute x={x}: p({x})={ans}.'
    if k==6:
        r=rng.randint(-5,7); q=rng.randint(2,8); # construct p(x)=(x-r)(x+q)+rem
        rem=rng.randint(-10,10); # p(r)=rem
        # x^2 +(q-r)x-rq+rem
        b=q-r; c=-r*q+rem
        return f'When p(x)=x² {b:+d}x {c:+d} is divided by x {(-r):+d}, what is the remainder?', rem, [r,c,rem+1], f'By the Remainder Theorem, the remainder is p({r})={rem}.'
    if k==7:
        x=rng.randint(2,12); a=rng.randint(2,7); b=rng.randint(1,7);
        while x==b: x=rng.randint(2,12)
        rhs=Fraction(a,x-b)
        return f'Solve for x: {a}/(x-{b}) = {fmt(rhs)}.', x, [x+b,x-b,b], f'Cross-multiplying and solving gives x={x}; x≠{b}.'
    if k==8:
        a=rng.randint(2,8); b=rng.randint(2,8); ans=f'{a}/(x+{b})'; orig=f'({a}x+{a*b})/((x+{b})²)'
        return f'For x ≠ -{b}, which expression is equivalent to {orig}?', ans, [f'{a}/x',f'1/(x+{b})',f'{a}/(x-{b})'], f'Factor the numerator as {a}(x+{b}) and cancel one common factor.'
    if k==9:
        a=rng.randint(2,6); m=rng.randint(2,5); n=rng.randint(2,5); ans=m+n
        return f'For x>0, x^{m} · x^{n} = x^k. What is k?', ans, [m*n,abs(m-n),ans+1], f'When multiplying equal bases, add exponents: k={m}+{n}={ans}.'
    if k==10:
        start=rng.choice([100,200,500,1000]); rate=rng.choice([5,10,20,25]); years=rng.choice([2,3]); val=start*((100+rate)/100)**years
        ans=round(val,2)
        return f'A quantity starts at {start} and increases by {rate}% each year. What is its value after {years} years?', ans, [round(start*(1+rate/100*years),2),round(val/(1+rate/100),2),start+rate*years], f'Use {start}(1+{rate}/100)^{years}={fmt(ans)}.'
    if k==11:
        start=rng.choice([400,800,1000,1200]); rate=rng.choice([10,20,25]); years=rng.choice([2,3]); val=start*((100-rate)/100)**years; ans=round(val,2)
        return f'A machine worth ${start} loses {rate}% of its value each year. What is its value after {years} years?', ans, [round(start*(1-rate/100*years),2),round(start*((100+rate)/100)**years,2),start-rate*years], f'Use exponential decay: {start}(1-{rate}/100)^{years}={fmt(ans)}.'
    if k==12:
        root=rng.randint(2,12); c=root*root; shift=rng.randint(-8,8); x=c-shift
        return f'If √(x {shift:+d}) = {root}, what is x?', x, [c,c+shift,x+root], f'Square both sides: x{shift:+d}={c}; x={x}.'
    if k==13:
        a=rng.randint(2,6); b=rng.randint(-5,5); c=rng.randint(2,6); d=rng.randint(-5,5); x=rng.randint(-4,6); gx=c*x+d; ans=a*gx+b
        return f'Let f(x)={a}x {b:+d} and g(x)={c}x {d:+d}. What is f(g({x}))?', ans, [a*x+b,c*(a*x+b)+d,gx], f'g({x})={gx}; then f({gx})={ans}.'
    if k==14:
        a=rng.randint(2,9); b=rng.randint(-12,12); y=rng.randint(-8,10); x=a*y+b
        return f'If f(x)={a}x {b:+d}, what is f⁻¹({x})?', y, [x,a*x+b,y+1], f'f⁻¹(z)=(z-{b})/{a}; substituting {x} gives {y}.'
    if k==15:
        x=rng.randint(-5,8); y=x*x+rng.randint(1,6); m=rng.randint(-4,4) or 2; b=y-m*x
        return f'The graphs y=x²+{y-x*x} and y={m}x {b:+d} intersect at x={x}. What is the corresponding y-coordinate?', y, [x,m*x,y+1], f'Substitute x={x} into either equation; y={y}.'
    if k==16:
        a=rng.randint(-8,8); b=rng.randint(-8,8); c=rng.randint(-8,8); d=rng.randint(-8,8); # (a+bi)+(c+di)
        real=a+c; imag=b+d; ans=f'{real}{imag:+d}i'
        return f'What is ({a}{b:+d}i)+({c}{d:+d}i)?', ans, [f'{a*c}{b*d:+d}i',f'{real}{b-d:+d}i',f'{a-c}{imag:+d}i'], f'Add real parts and imaginary parts: {real}{imag:+d}i.'
    if k==17:
        degree=rng.choice([3,4,5,6]); lead=rng.choice([2,3,-2,-3]); ans=degree
        return f'What is the degree of p(x)={lead}x^{degree} + {rng.randint(-5,5)}x² + {rng.randint(-5,5)}?', ans, [degree-1,2,abs(lead)], f'The highest exponent with a nonzero coefficient is {degree}.'
    if k==18:
        h=rng.randint(-6,6); ans=-h
        return f'The graph of y=f(x) is translated horizontally to produce y=f(x {h:+d}). By how many units is the graph shifted to the right (use a negative value for a shift left)?', ans, [h,abs(h),0], f'Replacing x by x+{h} shifts the graph {-h} units horizontally.'
    if k==19:
        r1=rng.randint(-7,7); r2=rng.randint(-7,7); s=r1+r2; p=r1*r2
        ans=s
        return f'The equation x² - kx {p:+d}=0 has roots {r1} and {r2}. What is k?', ans, [p,-s,abs(r1-r2)], f'For x²-kx+p, the sum of roots equals k. Thus k={s}.'
    # 20
    a=rng.randint(2,7); b=rng.randint(1,9); ans=a*a
    expr=f'({a}x+{b})² - {2*a*b}x - {b*b}'
    return f'Which expression is equivalent to {expr}?', f'{ans}x²', [f'{a}x²',f'{ans}x',f'{a*a+b*b}x²'], f'Expand the square; the linear and constant terms cancel, leaving {ans}x².'


def q_psda(i, rng):
    k=i+1
    if k==1:
        a,b=rng.sample(range(2,10),2); mult=rng.randint(3,12); total=(a+b)*mult; ans=a*mult
        return f'The ratio of red to blue cards is {a}:{b}. If there are {total} cards total, how many are red?', ans, [b*mult,mult,total-a], f'There are {a+b} ratio parts, each worth {mult}; red cards={a}×{mult}={ans}.'
    if k==2:
        a=rng.randint(2,8); b=rng.randint(3,10); x=rng.randint(5,20); y=Fraction(b*x,a)
        # ensure integer by set x multiple a
        x=a*rng.randint(2,8); y=b*x//a
        return f'If {a}/{b} = x/{y} and x={x}, what is y?', y, [x,b*x,y+b], f'Cross-multiply: {a}y={b}({x}), so y={y}.'
    if k==3:
        old=rng.choice([40,50,80,100,120,200]); pct=rng.choice([10,15,20,25,30,40]); new=old*(100+pct)//100
        return f'A value increases from {old} to {new}. What is the percent increase?', pct, [new-old,100*pct//(100+pct),pct+5], f'Percent increase=({new}-{old})/{old}×100={pct}%.'
    if k==4:
        n1=rng.randint(10,30); n2=rng.randint(10,30); avg1=rng.randint(60,85); avg2=rng.randint(70,95); ans=Fraction(n1*avg1+n2*avg2,n1+n2)
        return f'Class A has {n1} students with average {avg1}; Class B has {n2} students with average {avg2}. What is the combined average?', ans, [Fraction(avg1+avg2,2),avg1,avg2], f'Weighted average=({n1}·{avg1}+{n2}·{avg2})/({n1+n2})={fmt(ans)}.'
    if k==5:
        vals=sorted(rng.sample(range(5,60),5)); mean=Fraction(sum(vals),5); median=vals[2]
        ask=rng.choice(['mean','median']); ans=mean if ask=='mean' else median
        return f'For the data set {vals}, what is the {ask}?', ans, [median if ask=='mean' else mean,vals[0],vals[-1]], f'The {ask} is {fmt(ans)}.'
    if k==6:
        vals=sorted(rng.sample(range(10,100),6)); ans=vals[-1]-vals[0]
        return f'What is the range of the data set {vals}?', ans, [vals[-1],vals[0],ans+1], f'Range=max−min={vals[-1]}−{vals[0]}={ans}.'
    if k==7:
        total=rng.choice([20,30,40,50]); favorable=rng.randint(2,total-2); ans=Fraction(favorable,total)
        return f'A bag contains {total} equally likely tokens, {favorable} of which are marked A. What is the probability of selecting an A token?', ans, [Fraction(total-favorable,total),Fraction(favorable,total-favorable),Fraction(1,total)], f'Probability=favorable/total={favorable}/{total}={fmt(ans)}.'
    if k==8:
        # P(A|B)
        btotal=rng.choice([20,30,40]); both=rng.randint(2,btotal-2); ans=Fraction(both,btotal)
        return f'Among {btotal} students who take biology, {both} also take chemistry. If a biology student is chosen at random, what is the probability the student also takes chemistry?', ans, [Fraction(btotal-both,btotal),Fraction(both,btotal+10),Fraction(1,btotal)], f'Conditional probability is {both}/{btotal}={fmt(ans)}.'
    if k==9:
        a=rng.randint(10,40); b=rng.randint(10,40); c=rng.randint(10,40); d=rng.randint(10,40); row=a+b; ans=Fraction(a,row)
        return f'A two-way table has {a} group-1 students who passed and {b} group-1 students who did not pass. What fraction of group 1 passed?', ans, [Fraction(a,a+c),Fraction(a,a+b+c+d),Fraction(b,row)], f'Within group 1, total={row}; fraction passed={a}/{row}={fmt(ans)}.'
    if k==10:
        slope=rng.choice([-3,-2,-1,1,2,3,4]); ans='positive' if slope>0 else 'negative'
        return f'A scatterplot is well modeled by y={slope}x+{rng.randint(-10,10)}. What type of association does the model indicate?', ans, ['negative' if ans=='positive' else 'positive','no association','perfectly horizontal'], f'The regression slope is {slope}, so the association is {ans}.'
    if k==11:
        m=rng.randint(2,9); b=rng.randint(10,50); x=rng.randint(5,20); ans=m*x+b
        return f'A linear model predicts y={m}x+{b}. What value does the model predict when x={x}?', ans, [m*x,ans+m,ans-m], f'Substitute x={x}: y={m}({x})+{b}={ans}.'
    if k==12:
        sample=rng.randint(200,1200); pct=rng.choice([40,45,50,55,60,65]); pop=rng.choice([5000,10000,20000]); ans=round(pop*pct/100)
        return f'In a random sample of {sample} residents, {pct}% support a proposal. If the sample is representative of {pop} residents, about how many residents support it?', ans, [round(sample*pct/100),pop-ans,ans+sample], f'Estimate {pct}% of {pop}: {pct/100}×{pop}={ans}.'
    if k==13:
        estimate=rng.randint(40,70); moe=rng.choice([2,3,4,5]); low=estimate-moe; high=estimate+moe; ans=f'{low}% to {high}%'
        return f'A survey estimates {estimate}% support with a margin of error of ±{moe} percentage points. Which interval is consistent with the estimate?', ans, [f'{estimate}% to {high}%',f'{low-moe}% to {estimate}%',f'{estimate-moe//2}% to {estimate+moe//2}%'], f'Add and subtract the margin of error: {low}% to {high}%.'
    if k==14:
        miles=rng.randint(90,360); hours=rng.choice([2,3,4,5,6]); miles=(miles//hours)*hours; ans=miles//hours
        return f'A vehicle travels {miles} miles in {hours} hours at a constant rate. What is the unit rate in miles per hour?', ans, [miles, hours, ans+hours], f'Unit rate={miles}/{hours}={ans} miles per hour.'
    if k==15:
        mass=rng.randint(100,500); volume=rng.choice([20,25,40,50]); mass=(mass//volume)*volume; ans=mass//volume
        return f'A sample has mass {mass} grams and volume {volume} cubic centimeters. What is its density in grams per cubic centimeter?', ans, [mass*volume,volume/mass,ans+1], f'Density=mass/volume={mass}/{volume}={ans}.'
    if k==16:
        inches=rng.choice([24,36,48,60,72,84,96]); ans=inches/12
        return f'A board is {inches} inches long. How many feet long is it?', ans, [inches*12,inches/3,ans+1], f'12 inches=1 foot, so {inches}/12={fmt(ans)} feet.'
    if k==17:
        total=rng.choice([100,200,250,400,500]); part=rng.choice([20,25,40,50,75]); part=min(part,total//2); pct=100*part/total
        return f'A table reports {part} out of {total} observations in category A. What percentage is in category A?', pct, [100-pct,part,pct+10], f'Percentage={part}/{total}×100={fmt(pct)}%.'
    if k==18:
        start=rng.choice([100,200,500]); factor=rng.choice([1.1,1.2,1.5,2.0]); t=rng.choice([2,3,4]); ans=round(start*(factor**t),2)
        return f'A population is modeled by P(t)={start}({fmt(factor)})^t. What is P({t})?', ans, [round(start*factor*t,2),round(start*(factor**(t-1)),2),start+t], f'Substitute t={t}: P={start}({fmt(factor)})^{t}={fmt(ans)}.'
    if k==19:
        vals=sorted(rng.sample(range(5,80),8)); lower=vals[:4]; q1=Fraction(lower[1]+lower[2],2)
        return f'For the ordered data set {vals}, what is the first quartile Q1 using the median-of-halves method?', q1, [Fraction(vals[3]+vals[4],2),vals[1],vals[2]], f'The lower half is {lower}; its median is ({lower[1]}+{lower[2]})/2={fmt(q1)}.'
    # 20
    center=rng.randint(40,70); tight=[center-2,center-1,center,center+1,center+2]; wide=[center-10,center-5,center,center+5,center+10]
    ans='Data set B' if rng.choice([True,False]) else 'Data set A'
    # ensure question ordering
    if ans=='Data set B': A=tight; B=wide
    else: A=wide; B=tight
    return f'Data set A is {A} and data set B is {B}. Which data set has the greater standard deviation?', ans, ['Data set A' if ans=='Data set B' else 'Data set B','They are equal','Cannot be determined'], f'The data set with values farther from the common center has greater variability, so {ans} has the greater standard deviation.'


def q_geometry(i, rng):
    k=i+1
    if k==1:
        b=rng.randint(4,20); h=rng.randint(3,16); ans=Fraction(b*h,2)
        return f'A triangle has base {b} and height {h}. What is its area?', ans, [b*h,b+h,ans+1], f'Area=1/2·base·height=1/2·{b}·{h}={fmt(ans)}.'
    if k==2:
        l=rng.randint(4,20); w=rng.randint(3,15); ask=rng.choice(['area','perimeter']); ans=l*w if ask=='area' else 2*(l+w)
        return f'A rectangle has length {l} and width {w}. What is its {ask}?', ans, [2*(l+w) if ask=='area' else l*w,l+w,ans+2], f'The rectangle’s {ask} is {ans}.'
    if k==3:
        r=rng.randint(2,12); ans=f'{r*r}π'
        return f'A circle has radius {r}. What is its area in terms of π?', ans, [f'{2*r}π',f'{r}π',f'{2*r*r}π'], f'Area=πr²={r*r}π.'
    if k==4:
        r=rng.randint(2,12); ans=f'{2*r}π'
        return f'A circle has radius {r}. What is its circumference in terms of π?', ans, [f'{r*r}π',f'{r}π',f'{4*r}π'], f'Circumference=2πr={2*r}π.'
    if k==5:
        triples=[(3,4,5),(5,12,13),(8,15,17),(7,24,25),(9,40,41)]; a,b,c=rng.choice(triples); scale=rng.randint(1,4); a*=scale;b*=scale;c*=scale
        return f'A right triangle has legs {a} and {b}. What is the hypotenuse?', c, [a+b,c+scale,c-scale], f'By the Pythagorean theorem, c=√({a}²+{b}²)={c}.'
    if k==6:
        small=rng.randint(3,10); large=small*rng.randint(2,5); other=rng.randint(4,12); ans=other*large//small if (other*large)%small==0 else Fraction(other*large,small)
        return f'Two triangles are similar. A side of length {small} in the smaller triangle corresponds to a side of length {large} in the larger triangle. If another side of the smaller triangle is {other}, what is the corresponding larger side?', ans, [other+large-small,other*small/large,large], f'Scale factor={large}/{small}; corresponding side={other}·{large}/{small}={fmt(ans)}.'
    if k==7:
        angle=rng.randint(30,150); ans=180-angle
        return f'Two parallel lines are cut by a transversal. An interior angle on one side of the transversal measures {angle}°. What is the measure of the consecutive interior angle on the same side?', ans, [angle,90,180+angle], f'Same-side interior angles are supplementary: 180−{angle}={ans}°.'
    if k==8:
        n=rng.randint(4,12); ans=(n-2)*180
        return f'What is the sum of the interior angles of a {n}-sided polygon?', ans, [n*180,(n-1)*180,360], f'Sum=(n−2)180=({n}−2)180={ans}°.'
    if k==9:
        n=rng.choice([3,4,5,6,8,9,10,12]); ans=Fraction((n-2)*180,n)
        return f'What is the measure of each interior angle of a regular {n}-gon?', ans, [Fraction(360,n),180-ans,ans+10], f'Each angle=((n−2)180)/n={fmt(ans)}°.'
    if k==10:
        l=rng.randint(3,12); w=rng.randint(3,10); h=rng.randint(2,9); ans=l*w*h
        return f'A rectangular prism has dimensions {l}, {w}, and {h}. What is its volume?', ans, [l*w+w*h+l*h,l*w,2*(l*w+w*h+l*h)], f'Volume=lwh={l}·{w}·{h}={ans}.'
    if k==11:
        r=rng.randint(2,8); h=rng.randint(3,12); coeff=r*r*h; ans=f'{coeff}π'
        return f'A cylinder has radius {r} and height {h}. What is its volume in terms of π?', ans, [f'{2*r*h}π',f'{r*r}π',f'{2*coeff}π'], f'Volume=πr²h=π({r}²)({h})={coeff}π.'
    if k==12:
        r=rng.choice([3,6,9,12]); coeff=Fraction(4*r**3,3); ans=f'{fmt(coeff)}π'
        return f'A sphere has radius {r}. What is its volume in terms of π?', ans, [f'{r*r}π',f'{4*r*r}π',f'{fmt(Fraction(2*r**3,3))}π'], f'Volume=(4/3)πr³=(4/3)π({r}³)={fmt(coeff)}π.'
    if k==13:
        triple=rng.choice([(3,4,5),(5,12,13),(8,15,17)]); sx=rng.choice([-1,1]); sy=rng.choice([-1,1]); x1=rng.randint(-5,5); y1=rng.randint(-5,5); x2=x1+sx*triple[0]; y2=y1+sy*triple[1]; ans=triple[2]
        return f'What is the distance between ({x1}, {y1}) and ({x2}, {y2})?', ans, [triple[0]+triple[1],abs(x2-x1),abs(y2-y1)], f'Distance=√(({x2}-{x1})²+({y2}-{y1})²)={ans}.'
    if k==14:
        mx=rng.randint(-5,5); my=rng.randint(-5,5); dx=rng.randint(1,6); dy=rng.randint(1,6); x1=mx-dx;y1=my-dy;x2=mx+dx;y2=my+dy; ans=f'({mx}, {my})'
        return f'What is the midpoint of the segment from ({x1}, {y1}) to ({x2}, {y2})?', ans, [f'({x1}, {y1})',f'({x2}, {y2})',f'({dx}, {dy})'], f'Midpoint=((x₁+x₂)/2,(y₁+y₂)/2)=({mx},{my}).'
    if k==15:
        m=Fraction(rng.choice([1,2,3,4,5]),rng.choice([1,2,3,4])); ans=-1/m
        return f'A line has slope {fmt(m)}. What slope must a perpendicular line have?', ans, [m,-m,1/m], f'Perpendicular slopes are negative reciprocals, so the slope is {fmt(ans)}.'
    if k==16:
        triples=[(3,4,5),(5,12,13),(8,15,17),(7,24,25)]; opp,adj,hyp=rng.choice(triples); ask=rng.choice(['sin','cos','tan'])
        if ask=='sin': ans=Fraction(opp,hyp); formula='opposite/hypotenuse'
        elif ask=='cos': ans=Fraction(adj,hyp); formula='adjacent/hypotenuse'
        else: ans=Fraction(opp,adj); formula='opposite/adjacent'
        return f'In a right triangle relative to angle θ, the opposite side is {opp}, adjacent side is {adj}, and hypotenuse is {hyp}. What is {ask}(θ)?', ans, [Fraction(adj,hyp),Fraction(opp,adj),Fraction(adj,opp)], f'{ask}(θ)={formula}={fmt(ans)}.'
    if k==17:
        short=rng.randint(2,10); ans=f'{short}√3'
        return f'In a 30°-60°-90° triangle, the side opposite 30° has length {short}. What is the length of the side opposite 60°?', ans, [str(short*2),f'{short}√2',str(short)], f'The side ratio is 1:√3:2, so the longer leg is {short}√3.'
    if k==18:
        r=rng.randint(3,12); deg=rng.choice([30,45,60,90,120,180]); coeff=Fraction(deg,360)*2*r; ans=f'{fmt(coeff)}π'
        return f'A circle has radius {r}. What is the arc length subtended by a central angle of {deg}°, in terms of π?', ans, [f'{fmt(Fraction(deg,360)*r*r)}π',f'{2*r}π',f'{r}π'], f'Arc length=({deg}/360)·2π·{r}={fmt(coeff)}π.'
    if k==19:
        r=rng.randint(3,12); deg=rng.choice([30,45,60,90,120,180]); coeff=Fraction(deg,360)*r*r; ans=f'{fmt(coeff)}π'
        return f'A circle has radius {r}. What is the area of a sector with central angle {deg}°, in terms of π?', ans, [f'{fmt(Fraction(deg,360)*2*r)}π',f'{r*r}π',f'{2*r}π'], f'Sector area=({deg}/360)π({r}²)={fmt(coeff)}π.'
    # 20
    h=rng.randint(-6,6); k0=rng.randint(-6,6); r=rng.randint(2,9); ans=r
    return f'The circle (x {(-h):+d})² + (y {(-k0):+d})² = {r*r} has radius r. What is r?', ans, [r*r,abs(h),abs(k0)], f'In (x−h)²+(y−k)²=r², r²={r*r}; therefore r={r}.'


def generate_question(topic_idx, variant, salt=0):
    topic=TOPICS[topic_idx]
    rng=random.Random(f'v5-sat:{topic.code}:{variant}:{salt}')
    local=topic_idx%20
    if topic.domain=='Algebra': prompt,correct,ds,ex=q_algebra(local,rng)
    elif topic.domain=='Advanced Math': prompt,correct,ds,ex=q_advanced(local,rng)
    elif topic.domain=='Problem Solving and Data Analysis': prompt,correct,ds,ex=q_psda(local,rng)
    else: prompt,correct,ds,ex=q_geometry(local,rng)
    opts,correct_idx=options(correct,ds,rng)
    qid=str(uuid.uuid5(NS,f'{topic.code}:{variant}'))
    return {
        'id':qid,'topic_code':topic.code,'equivalent_no':variant,'domain':topic.domain,
        'prompt':prompt,'option_a':opts[0],'option_b':opts[1],'option_c':opts[2],'option_d':opts[3],
        'correct_option':'ABCD'[correct_idx],'correct_value':fmt(correct),'difficulty':topic.difficulty,
        'base_points':topic.points,'explanation':ex,'source':'original_generated_v5'
    }


def main():
    with (OUT/'topics.csv').open('w',newline='',encoding='utf-8') as f:
        w=csv.writer(f); w.writerow(['code','domain','name','difficulty','base_points','question_count'])
        for t in TOPICS: w.writerow([t.code,t.domain,t.name,t.difficulty,t.points,100])
    rows=[]
    seen_prompts=set()
    duplicate_fallbacks=0
    for ti in range(len(TOPICS)):
        topic=TOPICS[ti]
        for variant in range(1,101):
            row=None
            # Retry with deterministic salts so parameter-rich templates do not
            # accidentally produce exact duplicate prompts.
            for salt in range(80):
                candidate=generate_question(ti,variant,salt)
                if candidate['prompt'] not in seen_prompts:
                    row=candidate
                    break
            if row is None:
                # A few inherently finite templates (for example polygon-angle
                # facts) cannot yield 100 mathematically distinct stems. Keep the
                # content honest while giving each equivalent item a stable,
                # human-readable practice-form context rather than duplicating a
                # verbatim stem.
                row=generate_question(ti,variant,0)
                row['prompt']=f"Practice form {topic.code}-{variant:03d}: {row['prompt']}"
                duplicate_fallbacks += 1
            seen_prompts.add(row['prompt'])
            rows.append(row)
    fields=['id','topic_code','equivalent_no','domain','prompt','option_a','option_b','option_c','option_d','correct_option','correct_value','difficulty','base_points','explanation','source']
    with (OUT/'questions.csv').open('w',newline='',encoding='utf-8') as f:
        w=csv.DictWriter(f,fieldnames=fields); w.writeheader(); w.writerows(rows)
    # Public/student bank omits answer and rationale. Server bank above remains private.
    pub_fields=['id','topic_code','equivalent_no','domain','prompt','option_a','option_b','option_c','option_d','difficulty','base_points']
    with (OUT/'questions_public.csv').open('w',newline='',encoding='utf-8') as f:
        w=csv.DictWriter(f,fieldnames=pub_fields); w.writeheader();
        for r in rows: w.writerow({k:r[k] for k in pub_fields})
    qa={
        'version':'sat-math-v5.0.0','question_count':len(rows),'topic_count':len(TOPICS),
        'variants_per_topic':100,'domains':{},'unique_ids':len({r['id'] for r in rows}),
        'correct_option_errors':sum(r['correct_option'] not in 'ABCD' for r in rows),
        'duplicate_prompts':len(rows)-len({r['prompt'] for r in rows}),
        'practice_form_fallbacks':duplicate_fallbacks,
        'copyright':'Original programmatically generated SAT-style practice content; not official College Board material.'
    }
    for d in sorted({t.domain for t in TOPICS}): qa['domains'][d]=sum(r['domain']==d for r in rows)
    (OUT/'QA.json').write_text(json.dumps(qa,indent=2),encoding='utf-8')
    (OUT/'README.md').write_text('''# SAT Math V5 bank\n\n8,000 original SAT-style English-language math questions, generated as 80 topics × 100 equivalent variants.\n\n- `questions.csv`: private server bank including canonical answer and explanation.\n- `questions_public.csv`: answer-free bank useful for inspection only; production API still serves one question at a time.\n- `topics.csv`: topic metadata.\n- `QA.json`: generation QA summary.\n\nThis is original practice content and is **not official College Board material**.\n''',encoding='utf-8')
    print(json.dumps(qa,indent=2))

if __name__=='__main__': main()
