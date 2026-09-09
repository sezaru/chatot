package ui

// The emoji catalogue behind the composer's picker and the reaction picker,
// generated from the mockup (mockup/Chatot Interactive.dc.html) so the two
// cannot drift apart. Grouped the way Unicode groups them, because that is
// the order every other picker on the desktop uses, and every entry carries
// a name plus the words people actually type when they search for it
// ("lol", "thanks", "deal").
//
// One line per emoji: "<emoji> <name>:<extra search words>".
var emojiCatalogue = []emojiGroupSource{
	{Key: "smileys", Icon: "🙂", Label: "Smileys & emotion", Src: `
😀 grinning face:happy smile
😃 grinning face with big eyes:happy joy
😄 grinning face with smiling eyes:happy
😁 beaming face:grin
😆 grinning squinting face:laugh haha
😅 grinning face with sweat:phew relief
🤣 rolling on the floor laughing:rofl lol
😂 face with tears of joy:lol laugh cry
🙂 slightly smiling face:smile
🙃 upside-down face:silly irony
🫠 melting face:hot embarrassed
😉 winking face:wink flirt
😊 smiling face with smiling eyes:blush warm
😇 smiling face with halo:angel innocent
🥰 smiling face with hearts:love adore
😍 smiling face with heart-eyes:love crush
🤩 star-struck:amazed wow
😘 face blowing a kiss:love
😗 kissing face:kiss
😚 kissing face with closed eyes:kiss
😙 kissing face with smiling eyes:kiss
🥲 smiling face with tear:touched proud
😋 face savoring food:yum tasty
😛 face with tongue:cheeky
😜 winking face with tongue:joke
🤪 zany face:crazy goofy
😝 squinting face with tongue:yuck joke
🤑 money-mouth face:rich
🤗 smiling face with open hands:hug
🤭 face with hand over mouth:oops giggle
🫢 face with open eyes and hand over mouth:shock gasp
🤫 shushing face:quiet secret
🤔 thinking face:hmm doubt
🫡 saluting face:yes sir respect
🤐 zipper-mouth face:silence
🤨 face with raised eyebrow:suspicious really
😐 neutral face:meh
😑 expressionless face:blank
😶 face without mouth:speechless
🫥 dotted line face:invisible
😏 smirking face:smug
😒 unamused face:annoyed
🙄 face with rolling eyes:whatever
😬 grimacing face:awkward yikes
🤥 lying face:pinocchio
😌 relieved face:calm
😔 pensive face:sad down
😪 sleepy face:tired
🤤 drooling face:want
😴 sleeping face:zzz asleep
😷 face with medical mask:sick
🤒 face with thermometer:fever ill
🤕 face with head bandage:hurt
🤢 nauseated face:sick gross
🤮 face vomiting:sick
🤧 sneezing face:cold
🥵 hot face:heat
🥶 cold face:freezing
🥴 woozy face:drunk
😵 face with crossed-out eyes:dizzy knocked out
🤯 exploding head:mind blown
🤠 cowboy hat face:yeehaw
🥳 partying face:celebrate birthday
🥸 disguised face:incognito
😎 smiling face with sunglasses:cool
🤓 nerd face:geek
🧐 face with monocle:inspect
😕 confused face
🫤 face with diagonal mouth:unsure
😟 worried face
🙁 slightly frowning face:sad
😮 face with open mouth:wow surprised
😯 hushed face:surprised
😲 astonished face:shocked
😳 flushed face:embarrassed
🥺 pleading face:puppy eyes please
🥹 face holding back tears:emotional
😦 frowning face with open mouth
😧 anguished face
😨 fearful face:scared
😰 anxious face with sweat:nervous
😥 sad but relieved face
😢 crying face:sad tear
😭 loudly crying face:sob
😱 face screaming in fear:scream
😖 confounded face
😣 persevering face:struggle
😞 disappointed face
😓 downcast face with sweat
😩 weary face:ugh
😫 tired face:exhausted
🥱 yawning face:bored
😤 face with steam from nose:frustrated
😡 enraged face:angry mad
😠 angry face:mad
🤬 face with symbols on mouth:swearing
😈 smiling face with horns:devil mischief
👿 angry face with horns:devil
💀 skull:dead
☠️ skull and crossbones:danger
💩 pile of poo:crap
🤡 clown face
👻 ghost:boo halloween
👽 alien:ufo
🤖 robot:bot
😺 grinning cat
😹 cat with tears of joy:lol
😻 smiling cat with heart-eyes:love
😼 cat with wry smile
🙀 weary cat:shock
😿 crying cat
😾 pouting cat
❤️ red heart:love
🩷 pink heart:love
🧡 orange heart
💛 yellow heart
💚 green heart
💙 blue heart
💜 purple heart
🖤 black heart
🤍 white heart
🤎 brown heart
💔 broken heart:heartbreak
❤️‍🔥 heart on fire:passion
❤️‍🩹 mending heart:healing
💕 two hearts:love
💞 revolving hearts
💓 beating heart
💗 growing heart
💖 sparkling heart
💘 heart with arrow:cupid
💝 heart with ribbon:gift
💌 love letter
💤 zzz:sleep
💢 anger symbol:mad
💥 collision:boom
💫 dizzy:stars
💦 sweat droplets:splash
💨 dashing away:fast
💬 speech balloon:comment
💭 thought balloon:thinking
🗯️ right anger bubble:rage
`},
	{Key: "people", Icon: "👋", Label: "People & body", Src: `
👋 waving hand:hi bye hello
🤚 raised back of hand
🖐️ hand with fingers splayed
✋ raised hand:stop high five
🖖 vulcan salute:spock
🫱 rightwards hand
🫲 leftwards hand
🫳 palm down hand
🫴 palm up hand
👌 ok hand:perfect
🤌 pinched fingers:italian
🤏 pinching hand:small
✌️ victory hand:peace
🤞 crossed fingers:luck hope
🫰 fingers crossed thumb:heart money
🤟 love-you gesture
🤘 sign of the horns:rock
🤙 call me hand:shaka
👈 backhand index pointing left
👉 backhand index pointing right
👆 backhand index pointing up
👇 backhand index pointing down
☝️ index pointing up:one
🫵 index pointing at the viewer:you
👍 thumbs up:like yes ok agree
👎 thumbs down:dislike no
✊ raised fist:solidarity
👊 oncoming fist:punch bump
🤛 left-facing fist
🤜 right-facing fist
👏 clapping hands:applause bravo
🙌 raising hands:hooray praise
🫶 heart hands:love
👐 open hands
🤲 palms up together:pray
🤝 handshake:deal agree
🙏 folded hands:thanks please pray
✍️ writing hand:sign
💅 nail polish:manicure
🤳 selfie
💪 flexed biceps:strong gym
🦾 mechanical arm
🦵 leg
🦶 foot
👂 ear:listen
👃 nose:smell
🧠 brain:think
🫀 anatomical heart
🫁 lungs:breathe
🦷 tooth:dentist
🦴 bone
👀 eyes:look watching
👁️ eye:see
👅 tongue
👄 mouth:lips
👶 baby
🧒 child
👦 boy
👧 girl
🧑 person
👨 man
👩 woman
🧓 older person
👴 old man:grandpa
👵 old woman:grandma
🙋 person raising hand:question me
🤷 person shrugging:idk whatever
🤦 person facepalming:facepalm
🙆 person gesturing ok:yes
🙅 person gesturing no:stop nope
💁 person tipping hand:sassy info
🙇 person bowing:sorry thanks
🕺 man dancing:party
💃 woman dancing:party
🧑‍🤝‍🧑 people holding hands:friends
👫 woman and man holding hands:couple
👨‍👩‍👧 family
🫂 people hugging:hug support
👮 police officer:cop
👷 construction worker
💂 guard
🕵️ detective:spy
🧑‍⚕️ health worker:doctor nurse
🧑‍🍳 cook:chef
🧑‍🌾 farmer
🧑‍🔧 mechanic
🧑‍💻 technologist:developer coder
🧑‍🎤 singer:musician
🧑‍🚀 astronaut:space
🧑‍🚒 firefighter
🎅 santa claus:christmas
🧙 mage:wizard
🧚 fairy
🧛 vampire
🦸 superhero
🦹 supervillain
🧜 merperson:mermaid
`},
	{Key: "nature", Icon: "🐻", Label: "Animals & nature", Src: `
🐶 dog face:puppy
🐱 cat face:kitten
🐭 mouse face
🐹 hamster
🐰 rabbit face:bunny
🦊 fox
🐻 bear
🐼 panda
🐻‍❄️ polar bear
🐨 koala
🐯 tiger face
🦁 lion
🐮 cow face
🐷 pig face
🐸 frog
🐵 monkey face
🙈 see-no-evil monkey:shy oops
🙉 hear-no-evil monkey
🙊 speak-no-evil monkey:secret
🐒 monkey
🐔 chicken
🐧 penguin
🐦 bird
🐤 baby chick
🦆 duck
🦅 eagle
🦉 owl
🦇 bat
🐺 wolf
🐗 boar
🐴 horse face
🦄 unicorn
🐝 honeybee:bee
🐛 caterpillar:bug
🦋 butterfly
🐌 snail:slow
🐞 lady beetle:ladybug
🐜 ant
🕷️ spider
🦂 scorpion
🐢 turtle
🐍 snake
🦎 lizard
🦖 t-rex:dinosaur
🐙 octopus
🦑 squid
🦐 shrimp
🦀 crab
🐡 blowfish
🐠 tropical fish
🐟 fish
🐬 dolphin
🐳 spouting whale
🦈 shark
🐊 crocodile
🦓 zebra
🦍 gorilla
🐘 elephant
🦛 hippopotamus
🐪 camel
🦒 giraffe
🦌 deer
🐄 cow
🐖 pig
🐑 ewe:sheep
🐐 goat
🦔 hedgehog
🐇 rabbit
🐿️ chipmunk:squirrel
🦦 otter
🦥 sloth
🐕 dog
🐈 cat
🦮 guide dog
🕊️ dove:peace
🐾 paw prints
🌵 cactus
🎄 christmas tree
🌲 evergreen tree
🌳 deciduous tree
🌴 palm tree
🪴 potted plant
🌱 seedling:growth
🍀 four leaf clover:luck
🍃 leaf fluttering in wind
🍂 fallen leaf:autumn
🍁 maple leaf
🍄 mushroom
🌾 sheaf of rice:wheat
💐 bouquet:flowers
🌷 tulip
🌹 rose
🥀 wilted flower
🌺 hibiscus
🌸 cherry blossom:sakura
🌼 blossom
🌻 sunflower
🌞 sun with face
🌝 full moon face
🌚 new moon face
🌙 crescent moon:night
⭐ star
🌟 glowing star
✨ sparkles:shine magic
⚡ high voltage:lightning fast
☄️ comet
🔥 fire:lit hot
🌪️ tornado
🌈 rainbow
☀️ sun:sunny clear
🌤️ sun behind small cloud
⛅ sun behind cloud
☁️ cloud
🌧️ cloud with rain:rainy
⛈️ cloud with lightning and rain:storm
🌩️ cloud with lightning
🌨️ cloud with snow
❄️ snowflake:snow cold
☃️ snowman
💧 droplet:water
🌊 water wave:sea surf
🫧 bubbles
`},
	{Key: "food", Icon: "🍔", Label: "Food & drink", Src: `
🍏 green apple
🍎 red apple
🍐 pear
🍊 tangerine:orange
🍋 lemon
🍌 banana
🍉 watermelon
🍇 grapes
🍓 strawberry
🫐 blueberries
🍈 melon
🍒 cherries
🍑 peach
🥭 mango
🍍 pineapple
🥥 coconut
🥝 kiwi fruit
🍅 tomato
🍆 eggplant:aubergine
🥑 avocado
🥦 broccoli
🥬 leafy green
🥒 cucumber
🌶️ hot pepper:spicy chilli
🫑 bell pepper
🌽 corn
🥕 carrot
🫒 olive
🧄 garlic
🧅 onion
🥔 potato
🥐 croissant:bakery
🥯 bagel
🍞 bread
🥖 baguette
🥨 pretzel
🧀 cheese
🥚 egg
🍳 cooking:fried egg breakfast
🧈 butter
🥞 pancakes
🧇 waffle
🥓 bacon
🥩 cut of meat:steak
🍗 poultry leg:chicken
🍖 meat on bone
🌭 hot dog
🍔 hamburger:burger
🍟 french fries:chips
🍕 pizza
🫓 flatbread
🥪 sandwich
🌮 taco
🌯 burrito
🥙 stuffed flatbread:kebab
🧆 falafel
🥘 shallow pan of food:paella
🍲 pot of food:stew
🫕 fondue
🥣 bowl with spoon:cereal
🥗 green salad
🍿 popcorn:cinema
🧂 salt
🥫 canned food
🍱 bento box
🍙 rice ball
🍚 cooked rice
🍛 curry rice
🍜 steaming bowl:ramen noodles soup
🍝 spaghetti:pasta
🍠 roasted sweet potato
🍣 sushi
🍤 fried shrimp:tempura
🥟 dumpling
🦪 oyster
🍦 soft ice cream
🍧 shaved ice
🍨 ice cream
🍩 doughnut:donut
🍪 cookie:biscuit
🎂 birthday cake
🍰 shortcake:cake slice
🧁 cupcake
🥧 pie
🍫 chocolate bar
🍬 candy:sweets
🍭 lollipop
🍮 custard:flan
🍯 honey pot
🍼 baby bottle
🥛 glass of milk
☕ hot beverage:coffee tea
🫖 teapot
🍵 teacup:green tea
🍶 sake
🍾 bottle with popping cork:champagne celebrate
🍷 wine glass:wine
🍸 cocktail glass:martini
🍹 tropical drink:cocktail
🍺 beer mug:beer
🍻 clinking beer mugs:cheers
🥂 clinking glasses:cheers toast
🥃 tumbler glass:whisky
🧉 mate
🥤 cup with straw:soda
🧋 bubble tea
🧊 ice
🥢 chopsticks
🍽️ fork and knife with plate:dinner restaurant
🍴 fork and knife:eat
`},
	{Key: "activity", Icon: "⚽", Label: "Activity", Src: `
⚽ soccer ball:football
🏀 basketball
🏈 american football
⚾ baseball
🥎 softball
🎾 tennis
🏐 volleyball
🏉 rugby football
🥏 flying disc:frisbee
🎱 pool 8 ball:billiards
🏓 ping pong:table tennis
🏸 badminton
🏒 ice hockey
🏑 field hockey
🥍 lacrosse
🏏 cricket
🥅 goal net
⛳ flag in hole:golf
🪁 kite
🏹 bow and arrow:archery
🎣 fishing pole
🤿 diving mask
🥊 boxing glove
🥋 martial arts uniform:judo karate
🎽 running shirt
🛹 skateboard
🛼 roller skate
⛸️ ice skate
🥌 curling stone
🎿 skis
⛷️ skier
🏂 snowboarder
🏋️ person lifting weights:gym
🤸 person cartwheeling
⛹️ person bouncing ball
🤺 person fencing
🏌️ person golfing
🏇 horse racing
🧘 person in lotus position:yoga meditation
🏄 person surfing
🏊 person swimming
🚣 person rowing boat
🧗 person climbing
🚵 person mountain biking
🚴 person biking:cycling
🏆 trophy:win champion
🥇 1st place medal:gold win
🥈 2nd place medal:silver
🥉 3rd place medal:bronze
🏅 sports medal
🎖️ military medal
🎗️ reminder ribbon
🎫 ticket
🎟️ admission tickets
🎪 circus tent
🤹 person juggling
🎭 performing arts:theatre
🩰 ballet shoes
🎨 artist palette:art paint
🎬 clapper board:film
🎲 game die:dice
♟️ chess pawn
🎯 bullseye:dart target
🎳 bowling
🎮 video game:controller gaming
🕹️ joystick
🎰 slot machine
🧩 puzzle piece
🎁 wrapped gift:present
🎈 balloon
🎉 party popper:celebrate hooray
🎊 confetti ball:celebrate
🎀 ribbon
🎏 carp streamer
🏮 red paper lantern
🪅 piñata
🪩 mirror ball:disco
`},
	{Key: "travel", Icon: "🚗", Label: "Travel & places", Src: `
🚗 car:automobile drive
🚕 taxi:cab
🚙 sport utility vehicle:suv
🚌 bus
🏎️ racing car:formula
🚓 police car
🚑 ambulance
🚒 fire engine
🚐 minibus:van
🛻 pickup truck
🚚 delivery truck
🚛 articulated lorry
🚜 tractor
🛵 motor scooter
🏍️ motorcycle
🛺 auto rickshaw:tuk tuk
🚲 bicycle:bike
🛴 kick scooter
🦽 manual wheelchair
🦼 motorized wheelchair
🚨 police car light:siren alert
🚂 locomotive:train
🚆 train
🚇 metro:subway
🚊 tram
🚉 station
🚄 high-speed train
🚅 bullet train
🚡 aerial tramway:cable car
✈️ airplane:flight fly
🛫 airplane departure:takeoff
🛬 airplane arrival:landing
💺 seat
🚁 helicopter
🛰️ satellite
🚀 rocket:launch space
🛸 flying saucer:ufo
🛶 canoe
⛵ sailboat:sailing
🚤 speedboat
🛳️ passenger ship:cruise
⛴️ ferry
🚢 ship
⚓ anchor
⛽ fuel pump:petrol gas
🚧 construction:roadworks
🚦 vertical traffic light
🚥 horizontal traffic light
🗺️ world map:map
🗿 moai
🗽 statue of liberty
🗼 tokyo tower
🏰 castle
🏯 japanese castle
🏟️ stadium
🎡 ferris wheel
🎢 roller coaster
🎠 carousel horse
⛲ fountain
⛱️ umbrella on ground:beach
🏖️ beach with umbrella:holiday
🏝️ desert island
🏜️ desert
🌋 volcano
⛰️ mountain
🏔️ snow-capped mountain
🗻 mount fuji
🏕️ camping
⛺ tent
🏞️ national park
🛣️ motorway:highway
🛤️ railway track
🏠 house:home
🏡 house with garden
🏘️ houses
🏗️ building construction
🏭 factory
🏢 office building:work
🏬 department store
🏤 post office
🏥 hospital
🏦 bank
🏨 hotel
🏪 convenience store
🏫 school
💒 wedding
🏛️ classical building:museum
⛪ church
🕌 mosque
🕍 synagogue
🛕 hindu temple
⛩️ shinto shrine
🌁 foggy
🌃 night with stars
🏙️ cityscape:city
🌄 sunrise over mountains
🌅 sunrise
🌆 cityscape at dusk
🌇 sunset
🌉 bridge at night
♨️ hot springs:spa
🌌 milky way
🎆 fireworks
🎇 sparkler
🌠 shooting star
`},
	{Key: "objects", Icon: "💡", Label: "Objects", Src: `
⌚ watch
📱 mobile phone:smartphone
💻 laptop:computer
⌨️ keyboard
🖥️ desktop computer
🖨️ printer
🖱️ computer mouse
💾 floppy disk:save
💿 optical disk:cd
📀 dvd
🎥 movie camera
📽️ film projector
🎞️ film frames
📞 telephone receiver:call
☎️ telephone
📠 fax machine
📺 television:tv
📻 radio
🎙️ studio microphone:podcast
🎤 microphone:sing karaoke
🎧 headphone:music listen
🎼 musical score
🎵 musical note:music
🎶 musical notes:music song
🎹 musical keyboard:piano
🥁 drum
🎷 saxophone
🎺 trumpet
🪗 accordion
🎸 guitar
🪕 banjo
🎻 violin
🧭 compass
⏱️ stopwatch
⏲️ timer clock
⏰ alarm clock
🕰️ mantelpiece clock
⌛ hourglass done
⏳ hourglass:time waiting
📡 satellite antenna
🔋 battery
🪫 low battery
🔌 electric plug:power
💡 light bulb:idea
🔦 flashlight:torch
🕯️ candle
🧯 fire extinguisher
💸 money with wings:spend
💵 dollar banknote:money
💶 euro banknote:money
💷 pound banknote
💰 money bag:cash
🪙 coin
💳 credit card:pay
🧾 receipt:invoice
💎 gem stone:diamond
⚖️ balance scale:justice
🪜 ladder
🧰 toolbox
🪛 screwdriver
🔧 wrench:fix
🔨 hammer
🛠️ hammer and wrench:tools settings
🪚 saw
🔩 nut and bolt
⚙️ gear:settings
🧱 brick
⛓️ chains
🧲 magnet
💣 bomb
🧨 firecracker
🪓 axe
🔪 kitchen knife:cook
🛡️ shield:protect
⚰️ coffin
🏺 amphora
🔮 crystal ball:magic future
📿 prayer beads
🧿 nazar amulet
⚗️ alembic:chemistry
🔭 telescope
🔬 microscope
🩹 adhesive bandage:plaster
🩺 stethoscope
💊 pill:medicine
💉 syringe:vaccine
🩸 drop of blood
🧬 dna
🦠 microbe:virus
🧪 test tube
🌡️ thermometer:temperature
🧹 broom:clean
🧺 basket:laundry
🧻 roll of paper:toilet paper
🚽 toilet
🚿 shower
🛁 bathtub
🧼 soap
🪥 toothbrush
🧽 sponge
🪣 bucket
🧴 lotion bottle
🛎️ bellhop bell:service
🔑 key
🗝️ old key
🚪 door
🪑 chair
🛋️ couch and lamp:sofa
🛏️ bed:sleep
🧸 teddy bear:toy
🖼️ framed picture:art photo
🪞 mirror
🪟 window
🛍️ shopping bags:shopping
🛒 shopping cart:trolley
✉️ envelope:mail letter
📧 e-mail:email
📨 incoming envelope
📮 postbox
📦 package:parcel delivery
🏷️ label:tag price
📔 notebook with decorative cover
📕 closed book
📖 open book:read
📚 books:library study
📓 notebook
📜 scroll
📄 page facing up:document
📰 newspaper:news
🔖 bookmark
📈 chart increasing:growth up
📉 chart decreasing:down loss
📊 bar chart:stats poll
📅 calendar:date
📆 tear-off calendar
🗓️ spiral calendar
🗳️ ballot box:vote
📋 clipboard:list
📁 file folder
📂 open file folder
🗂️ card index dividers
🗒️ spiral notepad
📝 memo:note write
✏️ pencil:write edit
🖊️ pen
🖌️ paintbrush
🖍️ crayon
📌 pushpin:pin
📍 round pushpin:location
📎 paperclip:attach
📏 straight ruler
📐 triangular ruler
✂️ scissors:cut
🔒 locked:private secure
🔓 unlocked
🔐 locked with key:secure
🔍 magnifying glass:search find
👓 glasses
🕶️ sunglasses
🥼 lab coat
🦺 safety vest
👔 necktie
👕 t-shirt:clothes
👖 jeans
🧣 scarf
🧤 gloves
🧥 coat
🧦 socks
👗 dress
👘 kimono
👙 bikini
👛 purse
👜 handbag
🎒 backpack:school bag
👟 running shoe:sneaker
🥾 hiking boot
👠 high-heeled shoe
👢 woman's boot
👑 crown:king queen
🎩 top hat
🎓 graduation cap:study degree
🧢 billed cap
⛑️ rescue worker's helmet
💄 lipstick:makeup
💍 ring:engagement
`},
	{Key: "symbols", Icon: "🔣", Label: "Symbols", Src: `
✅ check mark button:done yes tick
☑️ check box with check:done
✔️ check mark:tick done
❌ cross mark:no wrong
❎ cross mark button
➕ plus:add
➖ minus
➗ divide
✖️ multiply
♾️ infinity
‼️ double exclamation
⁉️ exclamation question
❓ question mark:help
❗ exclamation mark:warning
〰️ wavy dash
💱 currency exchange
💲 heavy dollar sign
⚕️ medical symbol
♻️ recycling symbol:recycle eco
⚜️ fleur-de-lis
🔱 trident emblem
📛 name badge
⭕ hollow red circle
🚫 prohibited:no forbidden
⛔ no entry
📵 no mobile phones
🔞 no one under eighteen
☢️ radioactive
☣️ biohazard
⚠️ warning:caution
🚸 children crossing
♿ wheelchair symbol:accessible
🚭 no smoking
🛗 elevator:lift
🚹 mens room
🚺 womens room
🚻 restroom:toilet
🛂 passport control
🛃 customs
🛄 baggage claim
🛅 left luggage
⬆️ up arrow
↗️ up-right arrow
➡️ right arrow:next
↘️ down-right arrow
⬇️ down arrow:download
↙️ down-left arrow
⬅️ left arrow:back
↖️ up-left arrow
↕️ up-down arrow
↔️ left-right arrow
↩️ right arrow curving left:reply
↪️ left arrow curving right:forward
⤴️ right arrow curving up
⤵️ right arrow curving down
🔃 clockwise vertical arrows:refresh
🔄 counterclockwise arrows button:sync refresh
🔙 back arrow
🔚 end arrow
🔛 on arrow
🔜 soon arrow
🔝 top arrow
🛐 place of worship
⚛️ atom symbol:science
🕉️ om
✡️ star of david
☸️ wheel of dharma
☯️ yin yang
✝️ latin cross
☪️ star and crescent
☮️ peace symbol
🕎 menorah
🔯 dotted six-pointed star
♈ aries
♉ taurus
♊ gemini
♋ cancer
♌ leo
♍ virgo
♎ libra
♏ scorpio
♐ sagittarius
♑ capricorn
♒ aquarius
♓ pisces
🔀 shuffle tracks button:shuffle
🔁 repeat button:loop
🔂 repeat single button
▶️ play button:play
⏩ fast-forward
⏭️ next track
⏯️ play or pause
◀️ reverse button
⏪ fast reverse:rewind
⏮️ last track:previous
⏸️ pause button:pause
⏹️ stop button:stop
⏺️ record button:record
⏏️ eject button
🔅 dim button
🔆 bright button
📶 antenna bars:signal
📳 vibration mode
📴 mobile phone off
♀️ female sign
♂️ male sign
⚧️ transgender symbol
✳️ eight-spoked asterisk
✴️ eight-pointed star
❇️ sparkle
©️ copyright
®️ registered
™️ trade mark
#️⃣ keycap hash
*️⃣ keycap star
0️⃣ keycap 0 zero
1️⃣ keycap 1 one
2️⃣ keycap 2 two
3️⃣ keycap 3 three
4️⃣ keycap 4 four
5️⃣ keycap 5 five
6️⃣ keycap 6 six
7️⃣ keycap 7 seven
8️⃣ keycap 8 eight
9️⃣ keycap 9 nine
🔟 keycap 10 ten
🔠 input latin uppercase:abcd
🔡 input latin lowercase:abcd
🔤 input latin letters:abc
🆒 cool button
🆓 free button
ℹ️ information:info
🆔 id button
🆕 new button
🆗 ok button
🅿️ p button:parking
🆘 sos button:help
🆙 up button
🆚 vs button
🔴 red circle
🟠 orange circle
🟡 yellow circle
🟢 green circle:online
🔵 blue circle
🟣 purple circle
🟤 brown circle
⚫ black circle
⚪ white circle
🟥 red square
🟧 orange square
🟨 yellow square
🟩 green square
🟦 blue square
🟪 purple square
⬛ black large square
⬜ white large square
🔶 large orange diamond
🔷 large blue diamond
🔸 small orange diamond
🔹 small blue diamond
🔺 red triangle pointed up
🔻 red triangle pointed down
💠 diamond with a dot
🔘 radio button
🔳 white square button
🔲 black square button
`},
	{Key: "flags", Icon: "🏳️", Label: "Flags", Src: `
🏳️ white flag
🏴 black flag
🏁 chequered flag:race finish
🚩 triangular flag
🏳️‍🌈 rainbow flag:pride lgbt
🏳️‍⚧️ transgender flag
🏴‍☠️ pirate flag
🇵🇹 portugal:portuguese
🇧🇷 brazil:brasil
🇦🇴 angola
🇲🇿 mozambique
🇨🇻 cape verde
🇪🇸 spain
🇫🇷 france
🇩🇪 germany
🇮🇹 italy
🇬🇧 united kingdom:uk britain
🇮🇪 ireland
🇳🇱 netherlands
🇧🇪 belgium
🇨🇭 switzerland
🇦🇹 austria
🇸🇪 sweden
🇳🇴 norway
🇩🇰 denmark
🇫🇮 finland
🇵🇱 poland
🇺🇦 ukraine
🇬🇷 greece
🇹🇷 turkey
🇺🇸 united states:usa america
🇨🇦 canada
🇲🇽 mexico
🇦🇷 argentina
🇯🇵 japan
🇨🇳 china
🇰🇷 south korea
🇮🇳 india
🇦🇺 australia
🇳🇿 new zealand
🇿🇦 south africa
`},
}
