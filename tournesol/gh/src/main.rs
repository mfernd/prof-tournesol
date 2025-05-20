use std::net::SocketAddr;
use tokio::net::TcpListener;
use tracing::{error, info};

#[derive(Debug)]
pub enum ServerError {
    InvalidAddress(std::net::AddrParseError),
    TcpBind(std::io::Error),
    Run(std::io::Error),
}

#[tokio::main]
async fn main() -> Result<(), ServerError> {
    tracing_subscriber::fmt::init();

    let app = gh::create_root_app();

    let tcp_listener = get_tcp_listener().await?;
    let server = axum::serve(tcp_listener, app.into_make_service())
        .with_graceful_shutdown(shutdown_signal());
    info!("server starting");

    if let Err(e) = server.await {
        error!("server error: {:?}", e);
        return Err(ServerError::Run(e));
    }

    info!("server stopped");
    Ok(())
}

async fn get_tcp_listener() -> Result<TcpListener, ServerError> {
    let host = std::env::var("APP_HOST").unwrap_or_else(|_| String::from("0.0.0.0"));
    let port = std::env::var("APP_PORT").unwrap_or_else(|_| String::from("3000"));
    let addr: SocketAddr = format!("{}:{}", host, port)
        .parse()
        .map_err(ServerError::InvalidAddress)?;
    info!("binding to {}", addr);

    TcpListener::bind(addr).await.map_err(ServerError::TcpBind)
}

async fn shutdown_signal() {
    let ctrl_c = async {
        tokio::signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}
