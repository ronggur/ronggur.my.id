FROM nginx:1.27-alpine

COPY index.html flappy.html /usr/share/nginx/html/
COPY flappy-bird-assets/ /usr/share/nginx/html/flappy-bird-assets/

EXPOSE 80
