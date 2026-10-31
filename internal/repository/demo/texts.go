package demo

import "github.com/happyx-app/HappyX.Api/internal/domain"

// What the made-up team writes, in each language the demo is offered in.
type textSet struct {
	guildName string
	comments  map[domain.Mood][]string
	kudos     []string
	// questions[i] goes with answers[i], one answer per person.
	questions []string
	answers   [][]string
	proposals []string
}

// Languages are the demo's languages; the first is the default.
var Languages = []string{"en", "es"}

var texts = map[string]textSet{"es": es, "en": en}

var es = textSet{
	guildName: "Equipo Demo",
	comments: map[domain.Mood][]string{
		domain.MoodAwful: {
			"No he dormido nada y hoy no llego a todo.",
			"Semana muy dura, necesito parar un poco.",
			"Demasiadas reuniones, no he podido avanzar nada.",
			"Hoy me he sentido bastante solo con el problema.",
		},
		domain.MoodBad: {
			"Mucha presión con la entrega del viernes.",
			"Me cuesta concentrarme con tantas interrupciones.",
			"El despliegue ha fallado otra vez y estoy frustrado.",
			"Cansado, llevo varios días saliendo tarde.",
			"Hoy no me he sentido escuchado en la reunión.",
			"Demasiados cambios de prioridad esta semana.",
		},
		domain.MoodNeutral: {
			"Día normal, sin más.",
			"Bien, aunque con mucho trabajo pendiente.",
			"Algo cansado pero avanzando.",
			"Ni bien ni mal, día de tareas pequeñas.",
			"Esperando feedback para poder seguir.",
			"Me ha costado arrancar, pero vamos.",
		},
		domain.MoodGood: {
			"Buena reunión de equipo, por fin alineados.",
			"He podido cerrar varias tareas, contento.",
			"Me ha gustado el pair programming de hoy.",
			"Buen ambiente en la oficina.",
			"El cliente ha quedado contento con la demo.",
			"Día tranquilo y productivo.",
			"Gracias por la ayuda con el bug de ayer.",
		},
		domain.MoodGreat: {
			"¡Hemos lanzado la nueva versión! 🎉",
			"Comida de equipo genial, hacía falta.",
			"Me encanta el proyecto nuevo.",
			"Viernes y todo entregado a tiempo.",
			"Me siento muy apoyado por el equipo.",
		},
	},
	kudos: []string{
		"Gracias por quedarte a ayudarme con el despliegue.",
		"Tu presentación al cliente fue increíble.",
		"Gracias por revisar mi PR tan rápido.",
		"Siempre tienes un momento para explicar las cosas.",
		"Has salvado la demo de hoy, ¡gracias!",
		"Qué buen diseño el de la nueva pantalla.",
		"Gracias por cubrirme mientras estaba enfermo.",
		"Me encantó cómo llevaste la retro.",
		"Gracias por documentar todo tan bien.",
		"Tu idea para el onboarding es genial.",
		"Por estar siempre de buen humor, incluso los lunes.",
		"Gracias por el café y la charla, me hacía falta.",
		"El bug imposible era tuyo y lo has cazado. Crack.",
		"Gracias por organizar la comida de equipo.",
	},
	questions: []string{
		"¿Cuál fue tu primer trabajo?",
		"¿Qué canción no te cansas de escuchar?",
		"¿Qué harías con un año sabático?",
		"¿Cuál es tu comida favorita?",
		"¿Qué te gustaba hacer de pequeño?",
	},
	answers: [][]string{
		{
			"Socorrista en una piscina municipal", "Repartidor de pizzas en bici", "Monitor de campamento",
			"Camarero en un chiringuito", "Dependienta en una librería", "Profesor de repaso de mates",
			"Vendimia en el pueblo de mis abuelos", "Animador en un hotel", "Cajero en un supermercado",
			"Hacía webs para tiendas del barrio", "Paseador de perros", "Becario en una radio local",
		},
		{
			"Bohemian Rhapsody", "La Bachata, de Manuel Turizo", "Mr. Brightside", "Mediterráneo, de Serrat",
			"Despacito (no me juzguéis)", "Clocks, de Coldplay", "Lo que te conté mientras te hacías la dormida",
			"Dancing Queen", "Seven Nation Army", "Malamente, de Rosalía", "Wonderwall", "Take On Me",
		},
		{
			"Recorrer Sudamérica en mochila", "Aprender a hacer cerámica", "El Camino de Santiago entero",
			"Vivir un año en Japón", "Escribir una novela", "Montar una huerta en el pueblo",
			"Dar la vuelta al mundo en velero", "Aprender italiano en Florencia", "Voluntariado en un refugio de animales",
			"Hacer surf todos los días en Canarias", "Estudiar cocina en Perú", "Nada. Dormir. Mucho.",
		},
		{
			"La paella de mi madre", "Ramen", "Tortilla de patatas, con cebolla", "Pizza margarita",
			"Croquetas de jamón", "Tacos al pastor", "Sushi", "Lentejas de los lunes", "Hamburguesa con todo",
			"Pulpo a la gallega", "Arroz negro", "Cualquier cosa con chocolate",
		},
		{
			"Construir cabañas en el bosque", "Coleccionar cromos de fútbol", "Desmontar radios viejas",
			"Leer cómics de Mortadelo", "Jugar a la Game Boy debajo de las sábanas", "Hacer teatro en el cole",
			"Pescar con mi abuelo", "Patinar por todo el barrio", "Dibujar mapas de países inventados",
			"Montar en bici hasta que anochecía", "Hacer pasteles de barro", "Grabar vídeos con la cámara de mi padre",
		},
	},
	proposals: []string{
		"¿Cuál es la peor película que te ha gustado?",
		"¿Qué superpoder elegirías?",
		"¿Cuál es tu plan perfecto de domingo?",
	},
}

var en = textSet{
	guildName: "Demo Team",
	comments: map[domain.Mood][]string{
		domain.MoodAwful: {
			"Barely slept and can't keep up today.",
			"Really hard week, I need to slow down.",
			"Too many meetings, couldn't get anything done.",
			"Felt quite alone with the problem today.",
		},
		domain.MoodBad: {
			"Lots of pressure with Friday's release.",
			"Hard to focus with so many interruptions.",
			"The deploy failed again and I'm frustrated.",
			"Tired, I've been leaving late for days.",
			"I didn't feel heard in the meeting today.",
			"Too many priority changes this week.",
		},
		domain.MoodNeutral: {
			"Normal day, nothing special.",
			"Fine, though there's a lot left to do.",
			"A bit tired but making progress.",
			"Neither good nor bad, a day of small tasks.",
			"Waiting for feedback before I can go on.",
			"Slow start, but getting there.",
		},
		domain.MoodGood: {
			"Good team meeting, finally on the same page.",
			"Closed several tasks, happy.",
			"Loved today's pair programming.",
			"Nice atmosphere in the office.",
			"The client liked the demo.",
			"Quiet, productive day.",
			"Thanks for the help with yesterday's bug.",
		},
		domain.MoodGreat: {
			"We shipped the new version! 🎉",
			"Great team lunch, we needed it.",
			"I love the new project.",
			"Friday and everything delivered on time.",
			"I feel really supported by the team.",
		},
	},
	kudos: []string{
		"Thanks for staying late to help me with the deploy.",
		"Your presentation to the client was amazing.",
		"Thanks for reviewing my PR so fast.",
		"You always find a moment to explain things.",
		"You saved today's demo, thank you!",
		"What a nice design for the new screen.",
		"Thanks for covering for me while I was ill.",
		"Loved how you ran the retro.",
		"Thanks for documenting everything so well.",
		"Your onboarding idea is brilliant.",
		"For always being in a good mood, even on Mondays.",
		"Thanks for the coffee and the chat, I needed it.",
		"The impossible bug was yours and you caught it. Legend.",
		"Thanks for organising the team lunch.",
	},
	questions: []string{
		"What was your first job?",
		"Which song never gets old for you?",
		"What would you do with a year off?",
		"What's your favourite food?",
		"What did you love doing as a kid?",
	},
	answers: [][]string{
		{
			"Lifeguard at the local pool", "Pizza delivery by bike", "Summer camp instructor",
			"Waiter at a beach bar", "Bookshop assistant", "Maths tutor",
			"Grape harvest at my grandparents' village", "Hotel entertainer", "Supermarket cashier",
			"Building websites for local shops", "Dog walker", "Intern at a local radio",
		},
		{
			"Bohemian Rhapsody", "Here Comes the Sun", "Mr. Brightside", "Mediterráneo, by Serrat",
			"Despacito (don't judge me)", "Clocks, by Coldplay", "Africa, by Toto",
			"Dancing Queen", "Seven Nation Army", "Malamente, by Rosalía", "Wonderwall", "Take On Me",
		},
		{
			"Backpacking around South America", "Learning pottery", "Walking the whole Camino de Santiago",
			"Living in Japan for a year", "Writing a novel", "Starting a vegetable garden in the country",
			"Sailing around the world", "Learning Italian in Florence", "Volunteering at an animal shelter",
			"Surfing every day in the Canaries", "Studying cooking in Peru", "Nothing. Sleep. A lot.",
		},
		{
			"My mum's paella", "Ramen", "Spanish omelette, with onion", "Pizza margherita",
			"Ham croquettes", "Tacos al pastor", "Sushi", "Lentil stew on Mondays", "A burger with everything",
			"Galician-style octopus", "Black rice", "Anything with chocolate",
		},
		{
			"Building huts in the woods", "Collecting football stickers", "Taking old radios apart",
			"Reading comics", "Playing Game Boy under the covers", "School plays",
			"Fishing with my grandad", "Skating all over the neighbourhood", "Drawing maps of made-up countries",
			"Cycling until it got dark", "Making mud pies", "Filming videos with my dad's camera",
		},
	},
	proposals: []string{
		"What's the worst film you secretly love?",
		"Which superpower would you pick?",
		"What's your perfect Sunday?",
	},
}
