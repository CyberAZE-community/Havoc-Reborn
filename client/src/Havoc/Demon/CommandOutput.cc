#include <QJsonDocument>
#include <QJsonArray>

#include <Havoc/DemonCmdDispatch.h>

#include <UserInterface/Widgets/DemonInteracted.h>
#include <UserInterface/Widgets/TeamserverTabSession.h>
#include <UserInterface/Widgets/ProcessList.hpp>

#include <Util/ColorText.h>
#include <QFile>

using namespace HavocNamespace::HavocSpace;

void DispatchOutput::MessageOutput( QString JsonString, const QString& Date = "" ) const
{
    auto JsonDocument = QJsonDocument::fromJson( Util::base64_decode_capped( JsonString.toLocal8Bit( ) ) );
    auto TaskID       = JsonDocument[ "TaskID" ].toString();
    auto MessageType  = JsonDocument[ "Type" ].toString();
    auto Message      = JsonDocument[ "Message" ].toString();
    auto Output       = JsonDocument[ "Output" ].toString();


    if ( Message.length() > 0 )
    {
        if ( MessageType == "Error" || MessageType == "Erro" )
            this->DemonCommandInstance->DemonConsole->TaskError( Message );
        else if ( MessageType == "Good" )
            this->DemonCommandInstance->DemonConsole->AppendRaw( Util::ColorText::Green( "[+]" ) + " " + Message.toHtmlEscaped() );
        else if ( MessageType == "Info" )
            this->DemonCommandInstance->DemonConsole->AppendRaw( Util::ColorText::Cyan( "[*]" ) + " " + Message.toHtmlEscaped() );
        else if ( MessageType == "Warning" || MessageType == "Warn" )
            this->DemonCommandInstance->DemonConsole->AppendRaw( Util::ColorText::Yellow( "[!]" ) + " " + Message.toHtmlEscaped() );
        else
            this->DemonCommandInstance->DemonConsole->AppendRaw( Util::ColorText::Purple( "[^]" ) + " " + Message.toHtmlEscaped() );
    }

    if ( ! Output.isEmpty() )
    {
        //printf("task: %s\n", TaskID.toUtf8().constData());
        if (HavocX::callbackMessage)
        {
            auto GilState = PyGILState_Ensure();

            PyObject *arglist = Py_BuildValue( "s", Output.toUtf8().constData() );
            PyObject *result  = PyObject_CallFunctionObjArgs( HavocX::callbackMessage, arglist, NULL );
            Py_XDECREF( result );
            Py_XDECREF( arglist );
            Py_XDECREF( HavocX::callbackMessage );
            HavocX::callbackMessage = NULL;

            PyGILState_Release( GilState );
        }
        this->DemonCommandInstance->DemonConsole->AppendRaw( Output.toHtmlEscaped() );
    }

    if ( JsonDocument[ "MiscType" ].toString().compare( "" ) != 0 )
    {
        auto Type = JsonDocument[ "MiscType" ].toString();
        auto Data = JsonDocument[ "MiscData" ].toString();

        if ( Type.compare( "screenshot" ) == 0 )
        {
            auto DecodedData = Util::base64_decode_capped( Data.toLocal8Bit() );
            auto Name        = JsonDocument[ "MiscData2" ].toString();

            HavocX::Teamserver.TabSession->LootWidget->AddScreenshot( DemonCommandInstance->DemonID, Name, Date, DecodedData );
        }
        else if ( Type.compare( "download" ) == 0 )
        {
            auto MiscDataInfo = JsonDocument[ "MiscData2" ].toString().split( ";" );
            auto Name         = Util::base64_decode_capped( MiscDataInfo[ 0 ].toLocal8Bit() );
            auto Size         = ( MiscDataInfo[ 1 ] );

            HavocX::Teamserver.TabSession->LootWidget->AddDownload( DemonCommandInstance->DemonID, Name, Size, Date, nullptr );
        }
        else if ( Type.compare( "ProcessUI" ) == 0 )
        {
            for ( auto& Session : HavocX::Teamserver.Sessions )
            {
                if ( Session.Name == DemonCommandInstance->DemonID )
                {
                    if ( Session.ProcessList )
                    {
                        auto Decoded = Util::base64_decode_capped( Data.toLocal8Bit() );
                        Session.ProcessList->UpdateProcessListJson( QJsonDocument::fromJson( Decoded ) );
                    }
                }
            }
        }
        else if ( Type.compare( "FileExplorer" ) == 0 )
        {
            for ( auto& Session : HavocX::Teamserver.Sessions )
            {
                if ( Session.Name == DemonCommandInstance->DemonID )
                {
                    if ( Session.FileBrowser )
                    {
                        auto Decoded = Util::base64_decode_capped( Data.toLocal8Bit() );
                        Session.FileBrowser->AddData( QJsonDocument::fromJson( Decoded ) );
                    }
                }
            }
        }
        else if ( Type.compare( "disconnect" ) == 0 )
        {
            HavocX::Teamserver.TabSession->SessionGraphWidget->GraphPivotNodeDisconnect( Data );
        }
        else if ( Type.compare( "reconnect" ) == 0 )
        {
            auto Split = Data.split( ";" );

            HavocX::Teamserver.TabSession->SessionGraphWidget->GraphPivotNodeReconnect( Split[ 0 ], Split[ 1 ] );
        }
    }
}
