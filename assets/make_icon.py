import math, sys
from PIL import Image, ImageDraw, ImageFilter, ImageChops
S=1024
def squircle_mask(size, n=5.0, inset=0):
    # superellipse mask, supersampled
    m=Image.new('L',(size,size),0)
    px=m.load()
    r=(size/2)-inset
    c=size/2
    for y in range(size):
        dy=abs(y+0.5-c)/r
        if dy>1: continue
        # x extent
        v=1-dy**n
        if v<=0: continue
        dx=v**(1/n)*r
        x0=int(c-dx); x1=int(c+dx)
        for x in range(max(0,x0),min(size,x1+1)):
            px[x,y]=255
    return m.filter(ImageFilter.GaussianBlur(0.8))
def lerp(a,b,t): return tuple(int(a[i]+(b[i]-a[i])*t) for i in range(3))
def grad(size):
    img=Image.new('RGB',(size,size))
    px=img.load()
    top=(120,100,255); bot=(10,132,255)
    for y in range(size):
        for x in range(size):
            t=(y/size*0.75 + x/size*0.25)
            px[x,y]=lerp(top,bot,t)
    return img
base=1024
mask=squircle_mask(base,5.0,inset=40)
bg=grad(base)
icon=Image.new('RGBA',(base,base),(0,0,0,0))
icon.paste(bg,(0,0),mask)
# subtle top highlight and inner border
hl=Image.new('RGBA',(base,base),(0,0,0,0))
hd=ImageDraw.Draw(hl)
hd.ellipse([-200,-700,base+200,360],fill=(255,255,255,26))
hlc=Image.new('RGBA',(base,base),(0,0,0,0)); hlc.paste(hl,(0,0),mask)
icon=Image.alpha_composite(icon,hlc)
# overlay layer drawn then clipped by mask
ov=Image.new('RGBA',(base,base),(0,0,0,0))
d=ImageDraw.Draw(ov)
# soft glow orb in the middle
glow=Image.new('RGBA',(base,base),(0,0,0,0))
gd=ImageDraw.Draw(glow)
cx,cy,R=512,540,230
for i in range(40,0,-1):
    r=R*(1+i*0.035)
    a=int(6+ (40-i)*0.9)
    gd.ellipse([cx-r,cy-r,cx+r,cy+r],fill=(255,255,255,max(0,min(255,int(a*0.7)))))
glow=glow.filter(ImageFilter.GaussianBlur(14))
ov=Image.alpha_composite(ov,glow)
d=ImageDraw.Draw(ov)
# orb
orb=Image.new('RGBA',(base,base),(0,0,0,0))
od=ImageDraw.Draw(orb)
for i in range(120):
    t=i/119
    r=R*(1-t*0.0)
    col=lerp((255,250,220),(255,214,102),t)
od.ellipse([cx-R,cy-R,cx+R,cy+R],fill=(255,236,160,255))
# gradient inside orb
og=Image.new('RGBA',(base,base),(0,0,0,0))
ogp=og.load()
for y in range(cy-R,cy+R):
    for x in range(cx-R,cx+R):
        dd=math.hypot(x-cx,y-(cy-60))/ (R*1.1)
        if math.hypot(x-cx,y-cy)<=R:
            t=min(1,dd)
            c=lerp((255,255,240),(255,196,70),t)
            ogp[x,y]=c+(255,)
ov=Image.alpha_composite(ov,og)
d=ImageDraw.Draw(ov)
# menu bar (frosted white band)
d.rectangle([0,40,base,40+150],fill=(255,255,255,200))
# dots in the bar
for i,col in enumerate([(255,95,87),(254,188,46),(40,200,64)]):
    x=140+i*58
    d.ellipse([x-18,115-18,x+18,115+18],fill=col+(255,))
# apple-like menu items as small rounded bars on right
for i,w in enumerate([90,70,110]):
    x=520+i*130
    d.rounded_rectangle([x,100,x+w,130],radius=14,fill=(60,60,90,150))
# dock pill
d.rounded_rectangle([210,800,814,930],radius=62,fill=(255,255,255,170))
cols=[(255,95,87),(254,188,46),(40,200,64),(10,132,255),(175,82,222)]
for i,c in enumerate(cols):
    x=252+i*108
    d.rounded_rectangle([x,822,x+84,908],radius=24,fill=c+(255,))
# clip
clip=Image.new('RGBA',(base,base),(0,0,0,0))
clip.paste(ov,(0,0),mask)
icon=Image.alpha_composite(icon,clip)
icon.save(sys.argv[1])
